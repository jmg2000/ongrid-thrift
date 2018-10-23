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

	fmt.Printf("fileID: %s\n", vars["fileID"])
	fmt.Printf("userID: %s\n", vars["userID"])

	filePath := filepath.Join("files", vars["userID"], "attachments", vars["fileID"])
	fmt.Println(filePath)

	inFile, err := os.Open(filePath)
	if err != nil {
		log.Printf("AttachDownloadHandler: %v", err)
		w.WriteHeader(404)
		return
	}
	defer inFile.Close()

	io.Copy(w, inFile)

	log.Printf("vars: %v\n", vars)
}

// ResourceDownloadHandler ...
func ResourceDownloadHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	fmt.Printf("fileName: %s\n", vars["fileName"])
	fmt.Printf("userID: %s\n", vars["userID"])

	filePath := filepath.Join("files", vars["userID"], "resources", vars["fileName"])
	fmt.Println(filePath)

	inFile, err := os.Open(filePath)
	if err != nil {
		log.Printf("ResourceDownloadHandler: %v", err)
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

	vars := mux.Vars(r)
	userID := vars["userID"]

	log.Println("AttachUpload start")
	fmt.Printf("userID: %s\n", userID)

	err := r.ParseMultipartForm(int64(maxMemory))
	if err != nil {
		log.Println(err)
	}

	log.Printf("messageId = %s", r.FormValue("messageId"))

	f, h, err := r.FormFile("file")
	defer f.Close()

	log.Printf("file = %s, filesize = %d\n", h.Filename, h.Size)
	u4 := uuid.NewV4()
	filename := "files/" + userID + "/uploads/" + u4.String()
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

	route.HandleFunc("/files/{userID}/attachments/{fileID}", AttachDownloadHandler).Methods("GET")
	route.HandleFunc("/files/{userID}/resources/{fileName}", ResourceDownloadHandler).Methods("GET")
	route.HandleFunc("/attachments/{userID}", AttachUploadHandler).Methods("POST")

	if err := http.ListenAndServe(port, handlers.LoggingHandler(os.Stdout, route)); err != nil {
		log.Println("error running server:", err)
		return err
	}

	return nil
}
