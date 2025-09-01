package main

import (
	"fmt"
	"log"

	"encore.app/pkg/authn"
)

func main() {
	password := "admin123"
	hash, err := authn.HashPassword(password)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Hash for password 'admin123':")
	fmt.Println(hash)
}
