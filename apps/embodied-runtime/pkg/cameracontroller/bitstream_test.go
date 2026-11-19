package cameracontroller

import (
	"bytes"
	"testing"
)

// NAL header bytes for crafted H.264 test streams (4-byte start code + header).
//
//	0x67 = SPS (type 7), 0x68 = PPS (type 8), 0x65 = IDR (type 5),
//	0x41 = reference P-slice (type 1), 0x01 = non-reference P-slice (type 1).
var (
	sc4  = []byte{0, 0, 0, 1}
	sc3  = []byte{0, 0, 1}
	sps4 = append(append([]byte{}, sc4...), 0x67, 0x11, 0x22)
	pps4 = append(append([]byte{}, sc4...), 0x68, 0x33)
	idr4 = append(append([]byte{}, sc4...), 0x65, 0x44, 0x55, 0x66)
	ps4  = append(append([]byte{}, sc4...), 0x41, 0x77, 0x88)
	idr3 = append(append([]byte{}, sc3...), 0x65, 0x12, 0x34)
)

func concat(parts ...[]byte) []byte {
	var b bytes.Buffer
	for _, p := range parts {
		b.Write(p)
	}
	return b.Bytes()
}

func TestParseNALUnits(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want int // expected number of NAL units
	}{
		{"single 4-byte SC", sps4, 1},
		{"single 3-byte SC", idr3, 1},
		{"multi-NAL access unit (SPS+PPS+IDR)", concat(sps4, pps4, idr4), 3},
		{"PPS+IDR (matches camera sample)", concat(pps4, idr4), 2},
		{"no start code", []byte{1, 2, 3, 4}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNALUnits(tt.data)
			if len(got) != tt.want {
				t.Fatalf("got %d units, want %d: %v", len(got), tt.want, got)
			}
		})
	}
}

func TestParseNALUnits_ReconstructsInput(t *testing.T) {
	in := concat(sps4, pps4, idr4, ps4)
	var out bytes.Buffer
	for _, u := range parseNALUnits(in) {
		out.Write(u)
	}
	if !bytes.Equal(in, out.Bytes()) {
		t.Errorf("concatenated units != input")
	}
}

func TestNALTypes(t *testing.T) {
	in := concat(sps4, pps4, idr4, ps4)
	units := parseNALUnits(in)
	want := []byte{7, 8, 5, 1}
	if len(units) != len(want) {
		t.Fatalf("got %d units, want %d", len(units), len(want))
	}
	for i, u := range units {
		hdr, ok := nalHeaderByte(u)
		if !ok {
			t.Fatalf("unit %d: no header", i)
		}
		if got := h264NALType(hdr); got != want[i] {
			t.Errorf("unit %d type = %d, want %d", i, got, want[i])
		}
	}
}

func TestIsKeyframeNAL(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"P-slice (4-byte SC)", ps4, false},
		{"IDR (4-byte SC)", idr4, true},
		{"IDR (3-byte SC)", idr3, true},
		{"SPS only", sps4, true},
		{"PPS only", pps4, true},
		{"PPS+IDR (camera sample shape)", concat(pps4, idr4), true},
		{"SPS+PPS+IDR", concat(sps4, pps4, idr4), true},
		{"two P-slices", concat(ps4, ps4), false},
		{"empty", []byte{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isKeyframeNAL(tt.data, "h264"); got != tt.want {
				t.Errorf("isKeyframeNAL = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParamSetsBlob(t *testing.T) {
	t.Run("missing PPS", func(t *testing.T) {
		b := &paramSetsBlob{sps: sps4}
		if b.hasParamSets() {
			t.Error("hasParamSets should be false without PPS")
		}
		if b.paramPrefix() != nil {
			t.Error("paramPrefix should be nil without PPS")
		}
	})
	t.Run("complete", func(t *testing.T) {
		b := &paramSetsBlob{sps: sps4, pps: pps4}
		if !b.hasParamSets() {
			t.Error("hasParamSets should be true")
		}
		prefix := b.paramPrefix()
		want := concat(sps4, pps4)
		if !bytes.Equal(prefix, want) {
			t.Errorf("paramPrefix = %v, want %v", prefix, want)
		}
	})
}

func TestInjectBitstreamParams_CachesAndPrepends(t *testing.T) {
	var cs cameraState

	// Frame 1: SPS+PPS (no IDR) — cache populated, no prepend.
	f1 := &Frame{Data: concat(sps4, pps4), Encoding: "h264"}
	injectBitstreamParams(&cs, f1)
	blob := cs.paramSets.Load()
	if blob == nil || len(blob.sps) == 0 || len(blob.pps) == 0 {
		t.Fatal("param sets not cached")
	}
	if len(f1.Data) != len(sps4)+len(pps4) {
		t.Errorf("f1 data modified despite no IDR (len=%d)", len(f1.Data))
	}

	// Frame 2: bare IDR — prepend cached SPS+PPS so keyframe is self-contained.
	f2 := &Frame{Data: concat(idr4), Encoding: "h264"}
	injectBitstreamParams(&cs, f2)
	prefix := concat(sps4, pps4)
	if !bytes.HasPrefix(f2.Data, prefix) {
		t.Errorf("IDR not prepended with SPS/PPS")
	}
	if !bytes.HasSuffix(f2.Data, idr4) {
		t.Errorf("IDR payload not preserved after prepend")
	}
}

func TestInjectBitstreamParams_NoDoublePrepend(t *testing.T) {
	var cs cameraState
	au := concat(sps4, pps4, idr4) // access unit already carries SPS+PPS
	f := &Frame{Data: append([]byte{}, au...), Encoding: "h264"}
	injectBitstreamParams(&cs, f)
	if !bytes.Equal(f.Data, au) {
		t.Errorf("self-contained access unit was modified: %v", f.Data)
	}
}

func TestInjectBitstreamParams_PrependsWithoutIDR(t *testing.T) {
	var cs cameraState
	// Prime cache first.
	injectBitstreamParams(&cs, &Frame{Data: concat(sps4, pps4), Encoding: "h264"})
	// A P-slice must not be prepended (no IDR).
	f := &Frame{Data: concat(ps4), Encoding: "h264"}
	before := append([]byte{}, f.Data...)
	injectBitstreamParams(&cs, f)
	if !bytes.Equal(f.Data, before) {
		t.Errorf("P-slice was modified (should not prepend): %v", f.Data)
	}
}

func TestBuildPrimeFrame(t *testing.T) {
	blob := &paramSetsBlob{sps: sps4, pps: pps4}
	f := buildPrimeFrame(blob, "h264", 640, 480)
	if !bytes.Equal(f.Data, concat(sps4, pps4)) {
		t.Errorf("prime data = %v, want SPS+PPS", f.Data)
	}
	if f.Width != 640 || f.Height != 480 || f.Encoding != "h264" {
		t.Errorf("prime meta = %dx%d %q", f.Width, f.Height, f.Encoding)
	}
	// Prime must be flagged as a keyframe by the watch loop's detector.
	if !isKeyframeNAL(f.Data, "h264") {
		t.Error("prime frame not detected as keyframe")
	}
}

func TestDecodeHexNAL(t *testing.T) {
	// Bare NAL bytes → start code prepended.
	b, ok := decodeHexNAL("6711aa")
	if !ok || len(b) != 7 {
		t.Fatalf("got %v ok=%v", b, ok)
	}
	if !bytes.Equal(b, []byte{0, 0, 0, 1, 0x67, 0x11, 0xaa}) {
		t.Errorf("bare NAL not normalized: % x", b)
	}
	// Already has a 4-byte start code → unchanged.
	b, ok = decodeHexNAL("0000000168ab")
	if !ok || !bytes.Equal(b, []byte{0, 0, 0, 1, 0x68, 0xab}) {
		t.Errorf("with-SC NAL changed: % x", b)
	}
	// Spaces tolerated.
	b, ok = decodeHexNAL("00 00 00 01 65 cd")
	if !ok || b[4] != 0x65 {
		t.Errorf("spaced hex failed: % x", b)
	}
	// Empty / invalid.
	if _, ok := decodeHexNAL(""); ok {
		t.Error("empty should fail")
	}
	if _, ok := decodeHexNAL("zz"); ok {
		t.Error("invalid hex should fail")
	}
}

func TestSeedParamSets(t *testing.T) {
	var cs cameraState
	cfg := CameraConfig{
		ID: "cam0",
		Params: map[string]string{
			"sps": "67001122", // bare NAL → SC prepended
			"pps": "00000001 68aa",
		},
	}
	seedParamSets(&cs, cfg)
	blob := cs.paramSets.Load()
	if blob == nil {
		t.Fatal("param sets not seeded")
	}
	if !bytes.HasPrefix(blob.sps, sc4) || blob.sps[4] != 0x67 {
		t.Errorf("sps not normalized: % x", blob.sps)
	}
	if !bytes.HasPrefix(blob.pps, sc4) || blob.pps[4] != 0x68 {
		t.Errorf("pps not normalized: % x", blob.pps)
	}
	// hasParamSets true → primer would be built.
	if !blob.hasParamSets() {
		t.Error("seeded blob should satisfy hasParamSets")
	}
}

func TestSeedParamSets_MergesWithInBand(t *testing.T) {
	// Seed SPS only; camera supplies PPS in-band later.
	var cs cameraState
	seedParamSets(&cs, CameraConfig{ID: "c", Params: map[string]string{"sps": "67001122"}})

	// In-band frame carries PPS (no SPS). injectBitstreamParams must keep the
	// seeded SPS and add the in-band PPS (copy-on-write over the seed).
	injectBitstreamParams(&cs, &Frame{Data: concat(pps4), Encoding: "h264"})
	blob := cs.paramSets.Load()
	if blob == nil || len(blob.sps) == 0 || len(blob.pps) == 0 {
		t.Fatalf("merge failed: sps=%d pps=%d", len(blob.sps), len(blob.pps))
	}
	if blob.sps[4] != 0x67 {
		t.Errorf("seeded SPS lost after in-band update: % x", blob.sps)
	}
	if blob.pps[4] != 0x68 {
		t.Errorf("in-band PPS not cached: % x", blob.pps)
	}
}

func TestSeedParamSets_NoConfig(t *testing.T) {
	var cs cameraState
	seedParamSets(&cs, CameraConfig{ID: "c"}) // no params
	if cs.paramSets.Load() != nil {
		t.Error("should not seed when no param keys present")
	}
}
