// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/scrypt"
	"golang.org/x/term"
)

func readPassword(msg string) ([]byte, error) {
	if msg == "" {
		msg = "Password: "
	}
	fmt.Print(msg)

	// Disable echoing input to the terminal
	passwd, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return nil, fmt.Errorf("reading password failed: %w", err)
	}
	fmt.Println()

	return passwd, nil
}

func genEncKey(msg string, passwd []byte) (key []byte, err error) {
	if passwd == nil {
		passwd, err = readPassword(msg)
		if err != nil {
			return
		}
	}
	key, err = keyFromPassword(passwd)
	return
}

func keyFromPassword(passwd []byte) ([]byte, error) {
	hash := sha256.New()
	hash.Write(passwd)
	// userID would be admin for impersonation, so even if admin user knows
	// some other user's password, still admin cannot create the right key
	hash.Write([]byte(userID))
	salt := hash.Sum(nil)

	key, err := scrypt.Key(passwd, salt, 32768, 8, 1, 32)
	if err != nil {
		err = fmt.Errorf("generating key failed: %w", err)
		return nil, err
	}

	return key, nil
}

func keyFromEnv(name string) ([]byte, error) {
	passwd := os.Getenv(name)
	if len(passwd) == 0 {
		return nil, nil
	}
	return keyFromPassword([]byte(passwd))
}

func genEncKeyConfirm(readCur bool) (oldkey, newkey []byte, err error) {
	if readCur {
		oldkey, err = genEncKey("Current Password: ", nil)
		if err != nil {
			return
		}
	}
	passwd1, e := readPassword("New Password: ")
	if e != nil {
		err = e
		return
	}
	passwd2, e := readPassword("Retype New Password: ")
	if e != nil {
		err = e
		return
	}
	if !bytes.Equal(passwd1, passwd2) {
		err = errors.New("passwords do not match")
		return
	}
	newkey, err = genEncKey("", passwd1)
	return
}

func readLine(msg string) (string, error) {
	fmt.Printf("%s: ", msg)
	reader := bufio.NewReader(os.Stdin)
	result, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading from stdin failed: %w", err)
	}
	return strings.TrimSpace(result), nil
}

func askForConfirmation(message string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s [y/n]: ", message)

		response, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}

		response = strings.ToLower(strings.TrimSpace(response))

		switch response {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			return false, fmt.Errorf("unrecognized answer '%s'", response)
		}
	}
}
