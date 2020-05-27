package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"log"

	"golang.org/x/crypto/ripemd160"
)

const (
	checksumLength = 4
	// version 1
	version = byte(0x00)
)

// Wallet uses ecdsa (elyptical curve digital signing algorithm)
type Wallet struct {
	PrivateKey ecdsa.PrivateKey
	PublicKey  []byte
}

// Address returns the addess from the wallet. This includes the public key hash, the checksum and the version passed through a base58 algorithm
func (w *Wallet) Address() []byte {
	// generate the public hash key
	pubHash := PublicKeyHash(w.PublicKey)

	// attach the version to the hash
	versionedHash := append([]byte{version}, pubHash...)

	// create the checksum from the versioned hash
	checksum := Checksum(versionedHash)

	// append the checksum to get the full hash
	fullHash := append(versionedHash, checksum...)

	// pass the full hash through base 58 to remove bad characters
	address := Base58Encode(fullHash)

	return address
}

// NewKeyPair creates a new public private keypair 10^77 possibilities
func NewKeyPair() (ecdsa.PrivateKey, []byte) {
	// choose a p256 byte elliptic curve
	curve := elliptic.P256()

	// generate the private key
	// picks values along the ellitpic curve on random
	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Panic(err)
	}

	// create a public key by taking the value of x as bytes and explode the value of y into a single value
	pub := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...)
	return *private, pub
}

// MakeWallet creates a new wallet including key pairs
func MakeWallet() *Wallet {
	private, public := NewKeyPair()
	wallet := Wallet{private, public}

	return &wallet
}

// PublicKeyHash runs through several encryption algorithms to create the public key hash from a public key
func PublicKeyHash(pubKey []byte) []byte {
	pubHash := sha256.Sum256(pubKey)

	hasher := ripemd160.New()
	if _, err := hasher.Write(pubHash[:]); err != nil {
		log.Panic(err)
	}

	// we don't need to add bytes, we just want a slice of the current hasher bytes
	return hasher.Sum(nil)
}

// Checksum creates a checksum from the public key hash by running it through a sha256 twice, then returning the first 4 digits
func Checksum(payload []byte) []byte {

	firstHash := sha256.Sum256(payload)
	secondHash := sha256.Sum256(firstHash[:])

	return secondHash[:checksumLength]
}
