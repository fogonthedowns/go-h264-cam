package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const (
	readBufferSize    = 4096
	bufferSizeKB      = 512
	staticDir         = "static"
	staticURL         = "/static"
	videoWebsocketURL = "/stream"
	port              = 8080
	width             = 960
	height            = 540
	fps               = 30
)

var nalSeparator = []byte{0, 0, 0, 1} //NAL break

// Sender is a interface which implements the ability to transmit/broadcast messages
type Sender interface {
	Send([]byte) error
}

func startWebserver() {
	router := mux.NewRouter()

	// Websocket
	connectionNumber := make(chan int, 2)
	wsh := NewWebSocketHandler(connectionNumber)
	router.HandleFunc(videoWebsocketURL, wsh.Handler)
	go StreamVideo(width, height, fps, wsh, connectionNumber)

	// Static
	fs := http.FileServer(http.Dir(staticDir))
	router.PathPrefix(staticURL).Handler(handlers.CompressHandler(http.StripPrefix(staticURL, fs)))
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port), router))
}

// StreamVideo streams the video for the Raspberry Pi camera to a websocket
func StreamVideo(width int, height int, fps int, sender Sender, connectionsChange chan int) {
	stopChan := make(chan bool)
	cameraStarted := false

	for {
		select {
		case n := <-connectionsChange:
			if n <= 0 {
				stopChan <- true
				cameraStarted = false
			} else if !cameraStarted {
				go startCamera(width, height, fps, sender, stopChan)
				cameraStarted = true
			}
		}
	}

}

func startCamera(width int, height int, fps int, sender Sender, stop chan bool) {
	cmd := exec.Command("raspivid", "-ih", "-t", "0", "-o", "-", "-w", strconv.Itoa(width), "-h", strconv.Itoa(height), "-fps", strconv.Itoa(fps), "-n", "-pf", "baseline")
	fmt.Println("Started raspicam", cmd.Args)
	//defer cmd.Wait()
	//defer fmt.Println("Stopped raspicam")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	p := make([]byte, readBufferSize)
	buffer := make([]byte, bufferSizeKB*1024)
	currentPos := 0
	NALlen := len(nalSeparator)

	for {
		select {
		case <-stop:
			fmt.Println("Stop requested")
			return
		default:
			n, err := stdout.Read(p)
			if err != nil {
				if err == io.EOF {
					//fmt.Println(string(p[:n])) //should handle any remainding bytes.
					//return
				}
				fmt.Println(err)
				//os.Exit(1)
			}

			//fmt.Println("Received", p[:n])
			copied := copy(buffer[currentPos:], p[:n])
			startPosSearch := currentPos - NALlen
			endPos := currentPos + copied
			//fmt.Println("Buffer", buffer[:endPos])

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
				sender.Send(broadcast)

				// Shift
				copy(buffer, buffer[nalIndex:currentPos])
				currentPos = currentPos - nalIndex
			}
		}
	}
}

func main() {
	startWebserver()
}
