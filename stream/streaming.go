package stream

import (
	"bytes"
	"context"
	"io"
	"log"
	"os/exec"
	"sync"
)

const (
	readBufferSize = 4096
	bufferSizeKB   = 256

	// ffmpeg -f v4l2 -i /dev/video0 -c:v libx264 -f h264 -y output.h264
	h226cmd = "ffmpeg"
        maxRestartAttempts = 3
	legacyCommand    = "raspivid"
	libcameraCommand = "libcamera-vid"
)

var nalSeparator = []byte{0, 0, 0, 1} //NAL break

// CameraOptions sets the options to send to raspivid
type CameraOptions struct {
	Width               int
	Height              int
	Fps                 int
	HorizontalFlip      bool
	VerticalFlip        bool
	Rotation            int
	UseLibcamera        bool // Set to true to enable libcamera, otherwise use legacy raspivid stack
	AutoDetectLibCamera bool // Set to true to automatically detect if libcamera is available. If true, UseLibcamera is ignored.
}

// Video streams the video for the Raspberry Pi camera to a websocket
func Video(options CameraOptions, writer io.Writer, connectionsChange chan int) {
	log.Println("Entered Video function")
	stopChan := make(chan struct{})
	defer close(stopChan)
	cameraStarted := sync.Mutex{}
	firstConnection := true

	for n := range connectionsChange {
		log.Println("Number of connections changed to:", n)
		if n == 0 {
			firstConnection = true
			stopChan <- struct{}{}
		} else if firstConnection {
			firstConnection = false
			go startCamera(options, writer, stopChan, &cameraStarted)
		}
	}
}

func startCamera(options CameraOptions, writer io.Writer, stop <-chan struct{}, mutex *sync.Mutex) {
	log.Println("Starting the camera...")
	mutex.Lock()
	defer mutex.Unlock()
	defer log.Println("Stopped camera")
    restartAttempts := 0

        for restartAttempts < maxRestartAttempts {
		if err := runCamera(options, writer, stop); err != nil {
	        restartAttempts++
	        log.Printf("Camera error (attempt %d/%d): %v", restartAttempts, maxRestartAttempts, err)
        } else {
	    restartAttempts = 0 // reset if successful
	   }
	}

}

func runCamera(options CameraOptions, writer io.Writer, stop <-chan struct{}) error {
	args := []string{
			"-f", "v4l2",
			"-i", "/dev/video0",
			"-c:v", "libx264",
			"-f", "h264",
			"-an", // ignore audio
			 "-b:v", "50k",
			 "-preset", "veryfast",
			//"-preset veryfast",
			//"-b:v 1000k",
			//"-s 1280x720",
			"-s", "640x480",
			"-r", "15",
			"-",
		}

	command := determineCameraCommand(options)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, command, args...)
	defer cmd.Wait()
	defer cancel()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Println(err)
		return err
	}

	stderr, _ := cmd.StderrPipe()
        go func() {
		buf := make([]byte, 1024)
		for {
		n, _ := stderr.Read(buf)
		if n >0 {
			log.Printf("ffmpeg stderr: %s", buf[:n])
		}
          }
        }()

        if err := cmd.Start(); err != nil {
		log.Print(err)
		return err
	}
	log.Println("Started "+command, cmd.Args)

	p := make([]byte, readBufferSize)
	buffer := make([]byte, bufferSizeKB*1024)
	currentPos := 0
	NALlen := len(nalSeparator)

	for {
		select {
		case <-stop:
			log.Println("Stop requested")
			return nil
		default:
			n, err := stdout.Read(p)
			if err != nil {
				if err == io.EOF {
					log.Println("[" + command + "] EOF")
					return err
				}
				log.Println(err)
			}

			copied := copy(buffer[currentPos:], p[:n])
			startPosSearch := currentPos - NALlen
			endPos := currentPos + copied

			if startPosSearch < 0 {
				startPosSearch = 0
			}
			nalIndex := bytes.Index(buffer[startPosSearch:endPos], nalSeparator)

			currentPos = endPos
			if nalIndex > 0 {
				nalIndex += startPosSearch

				// Boadcast before the NAL
				broadcast := make([]byte, nalIndex)
				copy(broadcast, buffer)
				_, err := writer.Write(broadcast)
                                if err != nil {
					log.Printf("Error writing to websocket: %v", err)
				}

				// Shift
				copy(buffer, buffer[nalIndex:currentPos])
				currentPos = currentPos - nalIndex
			}
			if currentPos >= len(buffer) {
			   log.Println("warning: buffer full. restart mode ...")
			   currentPos = 0
			}
		}
	}
}

func determineCameraCommand(options CameraOptions) string {
	return h226cmd
	/*
	if options.AutoDetectLibCamera {
		_, err := exec.LookPath(libcameraCommand)
		if err == nil {
			return libcameraCommand
		}
		return legacyCommand
	}

	if options.UseLibcamera {
		return libcameraCommand
	} else {
		return legacyCommand
	}
	*/
}
