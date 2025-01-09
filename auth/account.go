package auth

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"golang.org/x/oauth2"
	"log"
	"os"
	"strings"
)

type Account map[string]*oauth2.Token

func ReadAccount(authName string, forceUpdate ...bool) (*oauth2.Token, error) {
	cache, err := os.ReadFile("account.dat")
	var account Account
	gob.NewDecoder(bytes.NewBuffer(cache)).Decode(&account)
	if accData, ok := account[authName]; ok && len(forceUpdate) == 0 {
		if accData.Valid() {
			return accData, nil
		}
	}
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
	}
	if username == "" {
		panic("Error: User Not Found")
	}
	if password == "" {
		panic("Error: Password Not Found")
	}
	accData, err := GetMCcredentialsByPassword(username, password)
	if err != nil {
		return nil, err
	}
	if account == nil {
		account = make(Account)
	}
	account[authName] = accData
	var b bytes.Buffer
	gob.NewEncoder(&b).Encode(account)
	err = os.WriteFile("account.dat", b.Bytes(), 0640)
	if err != nil {
		fmt.Println(err)
	}

	return accData, nil
}
