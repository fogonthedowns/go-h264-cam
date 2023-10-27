package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/bezineb5/go-h264-streamer/stream"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const (
	staticDir         = "static"
	staticURL         = "/static"
	videoWebsocketURL = "/stream"
	port              = 8080
	width             = 960
	height            = 540
	fps               = 30
)

func main() {
	log.Println("Starting the application...")
	options := stream.CameraOptions{
		Width:          width,
		Height:         height,
		Fps:            fps,
		HorizontalFlip: true,
		VerticalFlip:   true,
		Rotation:       0,
		UseLibcamera:   false,
	}

	router := mux.NewRouter()

	// Websocket
	connectionNumber := make(chan int, 2)
	wsh := NewWebSocketHandler(connectionNumber)
	router.HandleFunc(videoWebsocketURL, wsh.Handler)
	go stream.Video(options, wsh, connectionNumber)

	// Static
	fs := http.FileServer(http.Dir(staticDir))
	router.PathPrefix(staticURL).Handler(handlers.CompressHandler(http.StripPrefix(staticURL, fs)))
	log.Println("Server listening on port:", port)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(port), router))
}
