package auth

import (
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
)

func timeNow() time.Time { return time.Now() }

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func split3(s string) []string {
	return strings.SplitN(s, ".", 3)
}

func replaceSubstring(s, old, new string) string {
	return strings.Replace(s, old, new, 1)
}

func totpCode(secret string) (string, error) {
	return totp.GenerateCode(secret, timeNow())
}
