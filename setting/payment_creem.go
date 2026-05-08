package setting

import (
	"os"
	"strings"
)

var CreemApiKey = strings.TrimSpace(os.Getenv("CREEM_API_KEY"))
var CreemProducts = "[]"
var CreemTestMode = strings.EqualFold(strings.TrimSpace(os.Getenv("CREEM_TEST_MODE")), "true")
var CreemWebhookSecret = strings.TrimSpace(os.Getenv("CREEM_WEBHOOK_SECRET"))
