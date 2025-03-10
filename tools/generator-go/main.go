package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run . [train|test]")
		os.Exit(2)
	}
	mode := os.Args[1]
	switch mode {
	case "train":
		fmt.Println("generator: train mode stub")
	case "test":
		fmt.Println("generator: test mode stub")
	default:
		fmt.Println("unknown mode:", mode)
		os.Exit(2)
	}
}
