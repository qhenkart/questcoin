package wallet

import (
	"log"

	"github.com/mr-tron/base58"
)

// base58 uses 6 less characters than base64. It was invented along with Bitcoin
// missing 0 O 1 I + /  removed because of the visual confusion and we don't want users sending to the wrong address

// Base58Encode encodes a slice of bytes to base 58
func Base58Encode(input []byte) []byte {
	encode := base58.Encode(input)

	return []byte(encode)
}

// Base58Decode decodes a 58 base slice of bytes back to its original state
func Base58Decode(input []byte) []byte {
	decode, err := base58.Decode(string(input[:]))
	if err != nil {
		log.Panic(err)
	}

	return decode
}
