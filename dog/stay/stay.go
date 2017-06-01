package main

import (
	"C"

	"fmt"
)

// Dog is (wo)?man's best friend.
type Dog interface {
	// Name returns the name of the dog.
	Name() string

	// Self returns this instance.
	Self() Dog
}

// Command prints a dog's name to stdout.
func Command(d Dog) {
	fmt.Println(d.Name())
}
