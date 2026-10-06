package push

import webpush "github.com/SherClockHolmes/webpush-go"

// GenerateKeys makes a VAPID key pair, in the encoding push services and
// browsers read: the public key as an uncompressed P-256 point, both
// base64url without padding.
func GenerateKeys() (public, private string, err error) {
	private, public, err = webpush.GenerateVAPIDKeys()
	return public, private, err
}
