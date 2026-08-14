package cameracontroller

import (
	"encoding/hex"
	"log"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// H.264/H.265 Annex B bitstream helpers.
// ---------------------------------------------------------------------------

// paramSetsBlob caches the most recently observed parameter-set NAL units
// (VPS/SPS/PPS) for a bitstream source. It is stored atomically in
// cameraState.paramSets: written by the capture loop (which sees the stream
// from OpenCamera onward) and read by WatchFrames to prime late-joining
// subscribers that missed the camera's initial SPS/PPS.
type paramSetsBlob struct {
	vps []byte // H.265 VPS (NAL type 32); nil for H.264
	sps []byte // SPS (H.264 type 7 / H.265 type 33)
	pps []byte // PPS (H.264 type 8 / H.265 type 34)
}

// hasParamSets reports whether the blob has at least SPS and PPS, the
// minimum needed to decode any slice.
func (b *paramSetsBlob) hasParamSets() bool {
	return b != nil && len(b.sps) > 0 && len(b.pps) > 0
}

// paramPrefix returns the parameter-set NAL units concatenated in decode
// order (VPS, SPS, PPS), each including its start code. Returns nil if SPS
// or PPS is missing. The returned slice is freshly allocated and safe to
// prepend to a frame or send standalone.
func (b *paramSetsBlob) paramPrefix() []byte {
	if !b.hasParamSets() {
		return nil
	}
	var out []byte
	if len(b.vps) > 0 {
		out = append(out, b.vps...)
	}
	out = append(out, b.sps...)
	out = append(out, b.pps...)
	return out
}

// startCodeLen returns 4 if data[i:] begins with the 4-byte start code
// (00 00 00 01), 3 if it begins with the 3-byte start code (00 00 01),
// otherwise 0.
func startCodeLen(data []byte, i int) int {
	if i+3 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
		return 4
	}
	if i+2 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
		return 3
	}
	return 0
}

// parseNALUnits splits an Annex B byte-stream chunk into its NAL units. Each
// returned slice begins at the unit's start code (3- or 4-byte) and ends at
// the next start code (or end of data), so concatenating all units reproduces
// the input. Handles chunks containing multiple NAL units (e.g. an access
// unit with PPS+IDR). Returns nil if no start code is found.
//
// NAL bodies never contain 00 00 01 thanks to emulation prevention bytes
// (00 00 03), so scanning for start codes inside a body is safe.
func parseNALUnits(data []byte) [][]byte {
	var starts []int
	for i := 0; i+2 < len(data); {
		if sc := startCodeLen(data, i); sc != 0 {
			starts = append(starts, i)
			i += sc
			continue
		}
		i++
	}
	if len(starts) == 0 {
		return nil
	}
	units := make([][]byte, 0, len(starts))
	for idx, s := range starts {
		end := len(data)
		if idx+1 < len(starts) {
			end = starts[idx+1]
		}
		units = append(units, data[s:end])
	}
	return units
}

// nalHeaderByte returns the first byte after the start code of a NAL unit
// (the NAL header byte), which encodes the unit type.
func nalHeaderByte(unit []byte) (byte, bool) {
	sc := startCodeLen(unit, 0)
	if sc == 0 || len(unit) <= sc {
		return 0, false
	}
	return unit[sc], true
}

// h264NALType extracts the H.264 NAL unit type (lower 5 bits of the header).
func h264NALType(hdr byte) byte { return hdr & 0x1F }

// h265NALType extracts the H.265 NAL unit type (bits 1-6 of the header).
func h265NALType(hdr byte) byte { return (hdr >> 1) & 0x3F }

// frameContainsIDR reports whether any NAL unit in the chunk is an IDR
// (H.264 type 5; H.265 type 19 or 20).
func frameContainsIDR(units [][]byte, encoding string) bool {
	for _, u := range units {
		hdr, ok := nalHeaderByte(u)
		if !ok {
			continue
		}
		if encoding == "h264" {
			if h264NALType(hdr) == 5 {
				return true
			}
		} else if t := h265NALType(hdr); t == 19 || t == 20 {
			return true
		}
	}
	return false
}

// frameContainsParams reports whether the chunk already carries both SPS and
// PPS, used to avoid duplicating parameter sets when prepending.
func frameContainsParams(units [][]byte, encoding string) bool {
	hasSPS, hasPPS := false, false
	for _, u := range units {
		hdr, ok := nalHeaderByte(u)
		if !ok {
			continue
		}
		if encoding == "h264" {
			switch h264NALType(hdr) {
			case 7:
				hasSPS = true
			case 8:
				hasPPS = true
			}
		} else {
			switch h265NALType(hdr) {
			case 33:
				hasSPS = true
			case 34:
				hasPPS = true
			}
		}
	}
	return hasSPS && hasPPS
}

// seedParamSets seeds the cached parameter sets from the camera config so
// decoding works even when the device never emits SPS in-band — a common
// failing of some UVC H.264 cameras that only send PPS+IDR. Values are
// hex NAL units (with or without start code) under params keys "sps",
// "pps", and (for H.265) "vps". Obtainable from a one-shot capture or the
// device's UVC probe control. In-band updates (e.g. the camera's periodic
// PPS) merge on top of the seed via injectBitstreamParams.
func seedParamSets(cs *cameraState, cfg CameraConfig) {
	var blob paramSetsBlob
	if n, ok := decodeHexNAL(cfg.Param("sps", "")); ok {
		blob.sps = n
	}
	if n, ok := decodeHexNAL(cfg.Param("pps", "")); ok {
		blob.pps = n
	}
	if n, ok := decodeHexNAL(cfg.Param("vps", "")); ok {
		blob.vps = n
	}
	if blob.sps == nil && blob.pps == nil && blob.vps == nil {
		return
	}
	cs.paramSets.Store(&blob)
	log.Printf("[camera-controller] %s: seeded param sets from config (sps=%d pps=%d vps=%d bytes)",
		cfg.ID, len(blob.sps), len(blob.pps), len(blob.vps))
}

// decodeHexNAL decodes a hex-encoded NAL unit (spaces optional). The bytes
// may include the start code (00 00 00 01 / 00 00 01) or be the bare NAL; a
// 4-byte start code is prepended when absent so the result is a valid Annex B
// unit that can be prepended to a frame.
func decodeHexNAL(s string) ([]byte, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, " ", ""))
	if s == "" {
		return nil, false
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	if startCodeLen(b, 0) != 0 {
		return b, true
	}
	out := make([]byte, 4+len(b))
	out[0], out[1], out[2], out[3] = 0, 0, 0, 1
	copy(out[4:], b)
	return out, true
}

// injectBitstreamParams updates the cached parameter sets from any VPS/SPS/PPS
// NAL units in f and, if f contains an IDR, prepends the cached SPS/PPS so the
// keyframe is self-contained for any consumer (including late-joining
// WatchFrames subscribers).
//
// Must run in the capture loop before f is broadcast to subscribers.
func injectBitstreamParams(cs *cameraState, f *Frame) {
	units := parseNALUnits(f.Data)
	if len(units) == 0 {
		return
	}

	// Update cached parameter sets (copy-on-write over the current blob).
	var vps, sps, pps []byte
	cur := cs.paramSets.Load()
	hadPPS := cur != nil && len(cur.pps) > 0
	if cur != nil {
		vps, sps, pps = cur.vps, cur.sps, cur.pps
	}
	changed := false
	for _, u := range units {
		hdr, ok := nalHeaderByte(u)
		if !ok {
			continue
		}
		if f.Encoding == "h264" {
			switch h264NALType(hdr) {
			case 7:
				sps = copyFrameData(u)
				changed = true
			case 8:
				pps = copyFrameData(u)
				changed = true
			}
		} else {
			switch h265NALType(hdr) {
			case 32:
				vps = copyFrameData(u)
				changed = true
			case 33:
				sps = copyFrameData(u)
				changed = true
			case 34:
				pps = copyFrameData(u)
				changed = true
			}
		}
	}
	if changed {
		cs.paramSets.Store(&paramSetsBlob{vps: vps, sps: sps, pps: pps})
		log.Printf("[camera-controller] %s: cached param sets (sps=%d pps=%d vps=%d bytes)",
			cs.cfg.ID, len(sps), len(pps), len(vps))
		// First time we saw a PPS but still no SPS: the device emits PPS+IDR
		// without SPS, so decoding will fail until SPS is supplied out-of-band.
		if f.Encoding == "h264" && len(sps) == 0 && len(pps) > 0 && !hadPPS {
			log.Printf("[camera-controller] WARNING: %s emits PPS but no SPS in-band; "+
				"H.264 decode will fail — set params.sps (hex) in the camera config",
				cs.cfg.ID)
		}
	}

	// Make IDR keyframes self-contained by prepending cached SPS/PPS. Skip
	// if the access unit already carries both (the camera included them).
	if !frameContainsIDR(units, f.Encoding) || frameContainsParams(units, f.Encoding) {
		return
	}
	blob := cs.paramSets.Load()
	if blob == nil {
		return
	}
	prefix := blob.paramPrefix()
	if len(prefix) == 0 {
		return
	}
	out := make([]byte, 0, len(prefix)+len(f.Data))
	out = append(out, prefix...)
	out = append(out, f.Data...)
	f.Data = out
}

// buildPrimeFrame constructs a synthetic frame carrying the cached parameter
// sets, sent to a newly-registered WatchFrames subscriber before the live
// stream so it can decode the next IDR even after a late join.
func buildPrimeFrame(blob *paramSetsBlob, enc string, width, height int) *Frame {
	return &Frame{
		Data:      blob.paramPrefix(),
		Width:     width,
		Height:    height,
		Encoding:  enc,
		Timestamp: time.Now(),
	}
}
