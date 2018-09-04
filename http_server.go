package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// AttachDownloadHandler ...
func AttachDownloadHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	fmt.Printf("vars: %s\n", vars["fileID"])

	filePath := filepath.Join("attach", vars["fileID"])
	fmt.Println(filePath)

	inFile, err := os.Open(filepath.Join("attachments", vars["fileID"]))
	if err != nil {
		log.Printf("AttachDownloadHandler: %v", err)
		w.WriteHeader(404)
		return
	}
	defer inFile.Close()

	io.Copy(w, inFile)

	log.Printf("vars: %v\n", vars)
}

func runHTTPServer(port string) error {
	route := mux.NewRouter()

	route.HandleFunc("/attachments/{fileID}", AttachDownloadHandler).Methods("GET")

	if err := http.ListenAndServe(port, handlers.LoggingHandler(os.Stdout, route)); err != nil {
		log.Println("error running server:", err)
		return err
	}

	return nil
}
