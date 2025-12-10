package app

import (
	"crypto/rand"
	"maps"
	"time"

	"github.com/UNHCSC/pve-koth/auth"
	"github.com/gofiber/fiber/v2"
	"github.com/z46-dev/go-logger"
)

var (
	jwtSigningKey []byte         = make([]byte, 64)
	appLog        *logger.Logger = logger.NewLogger().SetPrefix("[APPL]", logger.BoldGreen)
)

func init() {
	if _, err := rand.Read(jwtSigningKey); err != nil {
		appLog.Errorf("failed to generate JWT signing key: %v\n", err)
		panic(err)
	}
}

func mustBeLoggedIn(c *fiber.Ctx) error {
	if auth.IsAuthenticated(c, jwtSigningKey) == nil {
		return c.Redirect("/login")
	}

	return c.Next()
}

func bindWithLocals(c *fiber.Ctx, binds fiber.Map) (out fiber.Map) {
	out = fiber.Map{
		"CurrentYear": time.Now().Year(),
	}

	if errMsg := c.Locals("Error"); errMsg != nil {
		out["Error"] = errMsg
	}

	maps.Copy(out, binds)

	return
}
