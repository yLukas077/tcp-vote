package main

import "log"

func main() {
	server := our_sever(":39902")

	log.Println("SERVIDOR ON")
	server.Start()
}