package main

import "net/http"

func main() {
	http.HandleFunc("/hi", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write([]byte("hello"))
	})

	http.HandleFunc("/bye", myHandlerFunc)

	http.ListenAndServe(":8080", nil)
}

func myHandlerFunc(rw http.ResponseWriter, r *http.Request) {
	rw.Write([]byte("goodbye"))
}
