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
	"github.com/satori/go.uuid"
)

// AttachDownloadHandler ...
func AttachDownloadHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	fmt.Printf("vars: %s\n", vars["fileID"])

	filePath := filepath.Join("attachments", vars["fileID"])
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

// AttachUploadHandler ...
func AttachUploadHandler(w http.ResponseWriter, r *http.Request) {
	maxMemory := 16 << 20

	log.Println("AttachUpload start")

	err := r.ParseMultipartForm(int64(maxMemory))
	if err != nil {
		log.Println(err)
	}

	log.Printf("messageId = %s", r.FormValue("messageId"))

	f, h, err := r.FormFile("file")
	defer f.Close()

	log.Printf("file = %s, filesize = %d\n", h.Filename, h.Size)
	u4 := uuid.NewV4()
	filename := "uploads/" + u4.String()
	log.Printf("filename: %s", filename)
	out, err := os.Create(filename)
	if err != nil {
		log.Println(err)
	}
	defer out.Close()
	io.Copy(out, f)

	w.WriteHeader(201)
	w.Write([]byte(u4.String()))
}

func runHTTPServer(port string) error {
	route := mux.NewRouter()

	route.HandleFunc("/attachments/{fileID}", AttachDownloadHandler).Methods("GET")
	route.HandleFunc("/attachments", AttachUploadHandler).Methods("POST")

	if err := http.ListenAndServe(port, handlers.LoggingHandler(os.Stdout, route)); err != nil {
		log.Println("error running server:", err)
		return err
	}

	return nil
}
