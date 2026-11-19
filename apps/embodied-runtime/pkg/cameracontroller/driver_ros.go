package cameracontroller

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"log"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// ROS topic driver — stub
// ---------------------------------------------------------------------------

// rospTopicDriver implements CameraDriver for ROS image topics.
// Since ROS image topics cannot be captured via ffmpeg directly, this
// driver is a placeholder that returns synthetic frames.
//
// TODO: Implement a real ROS subscriber using goroslib or similar.
//   - Subscribe to the configured ROS topic
//   - Decode incoming sensor_msgs/Image messages
//   - Convert to JPEG and push to the frame channel
type rospTopicDriver struct{}

func newROSTopicDriver() *rospTopicDriver {
	return &rospTopicDriver{}
}

// Open starts the ROS topic subscriber (stub).
func (d *rospTopicDriver) Open(ctx context.Context, cfg CameraConfig, width, height, fps int, encodingHint string) (FrameReader, string, error) {
	log.Printf("[camera-controller] ROS topic driver: %s (stub)", cfg.ID)
	return newSyntheticReader(cfg, width, height, fps), "jpeg", nil
}

// Close is a no-op.
func (d *rospTopicDriver) Close() error {
	return nil
}

// ---------------------------------------------------------------------------
// Synthetic frame reader (placeholder for drivers without real capture)
// ---------------------------------------------------------------------------

// syntheticReader generates synthetic frames at the configured FPS.
// Used as a placeholder for ROS topic and other stub drivers.
type syntheticReader struct {
	frames chan *Frame
	done   chan struct{}
	once   sync.Once
}

func newSyntheticReader(cfg CameraConfig, width, height, fps int) *syntheticReader {
	w, h, f := resolveResolution(cfg, width, height, fps)
	if f <= 0 {
		f = 15
	}

	r := &syntheticReader{
		frames: make(chan *Frame, 4),
		done:   make(chan struct{}),
	}

	go r.run(w, h, f)
	return r
}

func (r *syntheticReader) run(width, height, fps int) {
	defer close(r.frames)

	// Render a single valid black JPEG up front and reuse it for every
	// frame. Leaving Data nil would feed empty input to ffmpeg when a
	// consumer transcodes, which is undefined behavior.
	jpegData := generateSolidJPEG(width, height)

	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()

	var seq uint64
	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			seq++
			frame := &Frame{
				Data:      jpegData,
				Width:     width,
				Height:    height,
				Encoding:  "jpeg",
				Timestamp: time.Now(),
				Sequence:  seq,
			}
			select {
			case r.frames <- frame:
			default:
				// Drop if channel full.
			}
		}
	}
}

// generateSolidJPEG renders a solid black JPEG of the given size using only
// the standard library. It is used by stub readers so frame data is always
// non-nil and decodable by downstream transcoders.
func generateSolidJPEG(width, height int) []byte {
	if width <= 0 || height <= 0 {
		width, height = 1, 1
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 50}); err != nil {
		// Should never happen with an in-memory buffer; fall back to empty.
		return []byte{}
	}
	return buf.Bytes()
}

func (r *syntheticReader) Frames() <-chan *Frame {
	return r.frames
}

// Err is not used by the synthetic reader: it never fails on its own, it
// only stops via Close().
func (r *syntheticReader) Err() error { return nil }

func (r *syntheticReader) Close() error {
	r.once.Do(func() {
		close(r.done)
	})
	return nil
}
