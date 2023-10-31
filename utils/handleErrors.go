package utils

import (
	"fmt"
	"os"
)

func CheckErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occured: %v", err.Error())
		os.Exit(1)
	}
}
