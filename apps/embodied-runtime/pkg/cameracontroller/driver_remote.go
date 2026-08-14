package cameracontroller

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ---------------------------------------------------------------------------
// Remote camera driver — proxies to a camera on another gRPC server
// ---------------------------------------------------------------------------

// remoteDriver implements CameraDriver by forwarding all calls to a
// camera registered on a remote camera-controller gRPC server.
//
// Expected Params:
//
//	remote_address    — remote gRPC server address (e.g. "unix:///path/to/sock")
//	remote_camera_id  — the camera ID on the remote server
//	connect_timeout   — connection timeout (default: "10s")
type remoteDriver struct {
	remoteAddress  string
	remoteCameraID string
}

func newRemoteDriver(cfg CameraConfig) *remoteDriver {
	return &remoteDriver{
		remoteAddress:  cfg.Param("remote_address", ""),
		remoteCameraID: cfg.Param("remote_camera_id", cfg.ID),
	}
}

// Open connects to the remote server, opens the remote camera, and
// subscribes to its WatchFrames stream. The returned FrameReader just
// forwards each received VideoFrame as a Frame — no local capture, polling,
// or transcoding happens here; the remote server produces the requested
// encoding and this driver pipes it through verbatim.
func (d *remoteDriver) Open(ctx context.Context, cfg CameraConfig, width, height, fps int, encodingHint string) (FrameReader, string, error) {
	if d.remoteAddress == "" {
		return nil, "", fmt.Errorf("remote camera %q: remote_address param is required", cfg.ID)
	}

	timeoutStr := cfg.Param("connect_timeout", "10s")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 10 * time.Second
	}

	connCtx, cancel := context.WithTimeout(ctx, timeout)

	//nolint:staticcheck
	conn, err := grpc.DialContext(connCtx, d.remoteAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		cancel()
		return nil, "", fmt.Errorf("connect to remote camera-controller %q: %w", d.remoteAddress, err)
	}
	client := pb.NewCameraControllerClient(conn)

	// Open the remote camera.
	w, h, f := resolveResolution(cfg, width, height, fps)
	openReq := &pb.OpenCameraRequest{CameraId: d.remoteCameraID}
	if w > 0 {
		v := int32(w)
		openReq.Width = &v
	}
	if h > 0 {
		v := int32(h)
		openReq.Height = &v
	}
	if f > 0 {
		v := int32(f)
		openReq.Fps = &v
	}
	if encodingHint != "" {
		v := encodingHint
		openReq.Encoding = &v
	}

	openResp, err := client.OpenCamera(ctx, openReq)
	if err != nil {
		cancel()
		_ = conn.Close()
		return nil, "", fmt.Errorf("open remote camera %q: %w", d.remoteCameraID, err)
	}

	// The remote server reports the encoding it actually produces. We
	// subscribe to WatchFrames with that same encoding so the remote side
	// does a straight pass-through (no transcoding on either end).
	actualEnc := openResp.GetEncoding()
	if actualEnc == "" {
		actualEnc = encodingHint
	}

	// The stream context is independent of the gRPC client's context: it
	// lives until reader.Close() so a client disconnect does not tear down
	// the remote camera. Closing the reader cancels it.
	streamCtx, streamCancel := context.WithCancel(context.Background())
	stream, err := client.WatchFrames(streamCtx, &pb.WatchFramesRequest{
		CameraId: d.remoteCameraID,
	})
	if err != nil {
		streamCancel()
		// Best-effort close on the remote.
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = client.CloseCamera(closeCtx, &pb.CloseCameraRequest{CameraId: d.remoteCameraID})
		closeCancel()
		_ = conn.Close()
		cancel()
		return nil, "", fmt.Errorf("watch remote camera %q: %w", d.remoteCameraID, err)
	}

	// conn cleanup is owned by the reader now; release the dial cancel.
	cancel()

	log.Printf("[camera-controller] remote camera %q → %s camera %q (encoding=%s)",
		cfg.ID, d.remoteAddress, d.remoteCameraID, actualEnc)

	r := &remoteFrameReader{
		stream:   stream,
		client:   client,
		conn:     conn,
		cameraID: d.remoteCameraID,
		frames:   make(chan *Frame, 4),
		cancel:   streamCancel,
		stopped:  make(chan struct{}),
	}
	go r.loop()
	return r, actualEnc, nil
}

// Close is a no-op; the reader owns the connection and remote camera.
func (d *remoteDriver) Close() error {
	return nil
}

// ---------------------------------------------------------------------------
// remoteFrameReader — forwards the remote WatchFrames stream as Frames
// ---------------------------------------------------------------------------

// remoteFrameReader forwards VideoFrame messages received from the remote
// WatchFrames stream into a local channel of *Frame. It performs no local
// processing: the data, encoding, dimensions and timestamps all come from
// the remote server verbatim.
type remoteFrameReader struct {
	stream   pb.CameraController_WatchFramesClient
	client   pb.CameraControllerClient
	conn     *grpc.ClientConn
	cameraID string

	frames  chan *Frame
	cancel  context.CancelFunc
	once    sync.Once
	stopped chan struct{}
}

func (r *remoteFrameReader) Frames() <-chan *Frame { return r.frames }

// Err is not used by the remote reader: stream errors surface as a closed
// Frames() channel, and the remote server is responsible for reporting its
// own capture-source failures on its cameras' state.
func (r *remoteFrameReader) Err() error { return nil }

func (r *remoteFrameReader) Close() error {
	r.once.Do(func() {
		// Cancel the stream context so stream.Recv() returns immediately.
		r.cancel()
		// Wait for the forward loop to exit and close r.frames.
		<-r.stopped

		// Best-effort close on the remote.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = r.client.CloseCamera(ctx, &pb.CloseCameraRequest{CameraId: r.cameraID})
		cancel()
		if r.conn != nil {
			_ = r.conn.Close()
		}
	})
	return nil
}

// loop drains the remote stream into r.frames. It only blocks in Recv;
// sends to r.frames are non-blocking (slow consumers drop frames) so Close
// can always unblock it via the cancelled stream context.
func (r *remoteFrameReader) loop() {
	defer close(r.stopped)
	defer close(r.frames)

	var seq uint64
	for {
		msg, err := r.stream.Recv()
		if err != nil {
			// Stream ended (error, EOF, or context cancelled by Close).
			return
		}
		seq++
		frame := &Frame{
			Data:      msg.Data,
			Width:     int(msg.GetWidth()),
			Height:    int(msg.GetHeight()),
			Encoding:  msg.GetEncoding(),
			Timestamp: time.Unix(0, msg.GetTimestampNs()),
			Sequence:  seq,
		}
		select {
		case r.frames <- frame:
		default:
			// Drop if the consumer is slow; the next frame is more useful
			// than a stale one.
		}
	}
}
