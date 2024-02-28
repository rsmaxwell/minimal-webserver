package main

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	logger := log.New(os.Stdout, "", 0)

	hs := setup(logger)

	logger.Printf("Listening on http://0.0.0.0%s\n", hs.Addr)

	hs.ListenAndServe()
}

func setup(logger *log.Logger) *http.Server {
	return &http.Server{
		Addr:         getAddr(),
		Handler:      newServer(logWith(logger)),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func getAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}

	return ":8383"
}

func newServer(options ...Option) *Server {
	s := &Server{logger: log.New(io.Discard, "", 0)}

	for _, o := range options {
		o(s)
	}

	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/", s.index)

	return s
}

type Option func(*Server)

func logWith(logger *log.Logger) Option {
	return func(s *Server) {
		s.logger = logger
	}
}

type Server struct {
	mux    *http.ServeMux
	logger *log.Logger
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) log(format string, v ...interface{}) {
	s.logger.Printf(format+"\n", v...)
}

func inTrustedRoot(path string, trustedRoot string) error {
	for path != "/" {
		path = filepath.Dir(path)
		if path == trustedRoot {
			return nil
		}
	}
	return errors.New("path is outside of trusted root")
}

func verifyPathForGet(s *Server, raw_path string) (string, int, error) {

	working, err := os.Getwd()
	if err != nil {
		s.log("ERROR --> %d", http.StatusInternalServerError)
		s.log(err.Error())
		return "", http.StatusInternalServerError, err
	}

	trustedRoot := filepath.Join(working, "files")
	path := filepath.Join(working, "files", raw_path)

	cf := filepath.Clean(path)

	_, err = os.Stat(cf)
	if err != nil {
		s.log("ERROR --> %d", http.StatusNotFound)
		s.log(err.Error())
		return "", http.StatusNotFound, err
	}

	r, err := filepath.EvalSymlinks(cf)
	if err != nil {
		s.log("ERROR --> %d", http.StatusBadRequest)
		s.log(err.Error())
		return "", http.StatusBadRequest, err
	}

	err = inTrustedRoot(r, trustedRoot)
	if err != nil {
		s.log("ERROR --> %d", http.StatusBadRequest)
		s.log(err.Error())
		return "", http.StatusBadRequest, err
	}

	return r, http.StatusOK, nil
}

func verifyPathForPut(s *Server, raw_path string) (string, int, error) {

	working, err := os.Getwd()
	if err != nil {
		s.log("ERROR --> %d", http.StatusInternalServerError)
		s.log(err.Error())
		return "", http.StatusInternalServerError, err
	}

	trustedRoot := filepath.Join(working, "files")
	path := filepath.Join(working, "files", raw_path)

	cf := filepath.Clean(path)

	_, err = os.Stat(cf)
	if err == nil {
		s.log("ERROR --> %d", http.StatusBadRequest)
		s.log("file already exists")
		s.log(cf)
		return "", http.StatusBadRequest, err
	}

	err = inTrustedRoot(cf, trustedRoot)
	if err != nil {
		s.log("ERROR --> %d", http.StatusBadRequest)
		s.log(err.Error())
		return "", http.StatusBadRequest, err
	}

	cd := filepath.Dir(cf)
	err = os.MkdirAll(cd, 0666)
	if err != nil {
		s.log("ERROR --> %d", http.StatusBadRequest)
		s.log(err.Error())
		return "", http.StatusBadRequest, err
	}

	return cf, http.StatusOK, nil
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {

	// s.log("index: Request --> %+v", r)

	if r.Method == "GET" {

		filename, status, err := verifyPathForGet(s, r.URL.Path)
		if err != nil {
			w.WriteHeader(status)
			return
		}

		dat, err := os.ReadFile(filename)
		if err != nil {
			s.log("ERROR --> %d", http.StatusInternalServerError)
			s.log(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		s.log("Success! --> %d, (%d bytes)", http.StatusOK, len(dat))
		w.Write(dat)

	} else if r.Method == "PUT" {

		s.log("index: r.URL.Path: %s", r.URL.Path)

		filename, status, err := verifyPathForPut(s, r.URL.Path)
		if err != nil {
			w.WriteHeader(status)
			return
		}

		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			s.log("ERROR --> %d", http.StatusInternalServerError)
			s.log(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// s.log("Body: %s", reqBody)

		f, err := os.Create(filename)
		if err != nil {
			s.log("ERROR --> %d", http.StatusInternalServerError)
			s.log(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer f.Close()

		err = binary.Write(f, binary.BigEndian, reqBody)
		if err != nil {
			s.log("ERROR --> %d", http.StatusInternalServerError)
			s.log(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		s.log("Success! --> %d", http.StatusOK)
		w.WriteHeader(http.StatusOK)

	} else {
		s.log("ERROR --> %d", http.StatusMethodNotAllowed)
		s.log("Method:" + r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}
