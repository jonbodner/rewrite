package main

import (
	"github.com/go-chi/chi/v5"
	_ "github.com/jonbodner/orchestrion/instrument"
	"net/http"
)

var bye = "/bye"

func main() {
	r := chi.NewRouter()

	r.Get(bye, myHandlerFunc)

	r.Get("/hi", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("hello"))
	})

	r.Delete("/done", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusNoContent)
	})

	http.ListenAndServe(":8080", r)
}

func myHandlerFunc(rw http.ResponseWriter, r *http.Request) {
	rw.Write([]byte("goodbye"))
}
