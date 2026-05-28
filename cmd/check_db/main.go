package main

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgresql://postgres.hcbautmuykbatbzcrgnd:bd5q4qJt__33445@aws-1-ap-northeast-1.pooler.supabase.com:6543/postgres?sslmode=require"
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	var users []struct {
		Name     string  `db:"name"`
		Email    string  `db:"email"`
		FcmToken *string `db:"fcm_token"`
	}

	err = db.Select(&users, "SELECT name, email, fcm_token FROM users")
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	fmt.Println("List of registered users:")
	for _, u := range users {
		tokenStr := "NULL"
		if u.FcmToken != nil {
			tokenStr = *u.FcmToken
		}
		fmt.Printf("- Name: %s | Email: %s | FCM Token: %s\n", u.Name, u.Email, tokenStr)
	}
}
