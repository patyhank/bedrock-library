package auth

import (
	"golang.org/x/oauth2"
	"log"
	"os"
	"strings"
)

func ReadAccount(authName string) (*oauth2.Token, error) {
	var username string
	var password string
	file, err := os.ReadFile("accounts.txt")
	if err != nil {
		log.Fatalf("error reading accounts: %v", err)
	}
	accounts := strings.Split(strings.ReplaceAll(string(file), "\r", ""), "\n")

	for _, account := range accounts {
		if account == "" {
			continue
		}
		authNameIndex := strings.Index(account, " ")
		if authNameIndex == -1 {
			continue
		}
		if account[:authNameIndex] != authName {
			continue
		}
		pair := account[authNameIndex+1:]
		A := strings.Index(pair, ":")
		username = pair[:A]
		password = pair[A+1:]
		//fmt.Println(pair[A+1:])
	}
	if username == "" {
		panic("Error: User Not Found")
	}
	if password == "" {
		panic("Error: Password Not Found")
	}

	return GetMCcredentialsByPassword(username, password)
}
