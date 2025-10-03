package main

import (
	"fmt"
	"log"

	"encore.app/pkg/authn"
)

func main() {
	// توليد مفتاح تشفير آمن للـ AuditEncryptionKey
	key, err := authn.GenerateSecureSecret()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("مفتاح التشفير الآمن للـ AuditEncryptionKey:")
	fmt.Println(key)
	fmt.Println("\nاستخدم هذا المفتاح مع الأمر:")
	fmt.Printf("encore secret set --env=development AuditEncryptionKey %s\n", key)
	fmt.Printf("encore secret set --env=production AuditEncryptionKey %s\n", key)
}
