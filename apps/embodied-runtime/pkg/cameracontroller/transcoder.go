package cameracontroller

// ---------------------------------------------------------------------------
// Bitstream helpers (WatchFrames pass-through)
// ---------------------------------------------------------------------------

// isKeyframeNAL checks whether a chunk of Annex B data contains an IDR
// access unit (keyframe) or its parameter sets. It scans every NAL unit in
// the chunk, handling both 3-byte (00 00 01) and 4-byte (00 00 00 01) start
// codes and chunks containing multiple NAL units.
//
// For H.264, flags NAL type 5 (IDR), 7 (SPS), or 8 (PPS).
// For H.265, flags NAL type 19 (IDR_W_RADL), 20 (IDR_N_LP), 32 (VPS),
// 33 (SPS), or 34 (PPS).
//
// WatchFrames streams bitstream sources (h264/h265) verbatim; this helper lets
// the client identify IDR access points and parameter sets without decoding.
func isKeyframeNAL(data []byte, encoding string) bool {
	for _, unit := range parseNALUnits(data) {
		hdr, ok := nalHeaderByte(unit)
		if !ok {
			continue
		}
		if encoding == "h264" {
			switch h264NALType(hdr) {
			case 5, 7, 8: // IDR, SPS, PPS
				return true
			}
		} else {
			switch h265NALType(hdr) {
			case 19, 20, 32, 33, 34: // IDR_W_RADL, IDR_N_LP, VPS, SPS, PPS
				return true
			}
		}
	}
	return false
}
