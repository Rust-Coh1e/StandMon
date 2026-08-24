package main

import (
	"log"
	"net/http"

	"PriceMon/internal/handler"
	"PriceMon/internal/registry"
)

func main() {
	reg := registry.NewParserRegistry()

	parseHandler := handler.NewParseHandler(reg)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /parse", parseHandler.Parse)

	log.Println("server started on :8080")

	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		log.Fatal(err)
	}
}
