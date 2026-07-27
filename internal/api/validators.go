package api

import (
	"regexp"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// twPhonePattern matches Taiwan local mobile numbers, e.g. "0912345678".
var twPhonePattern = regexp.MustCompile(`^09\d{8}$`)

// registerCustomValidators adds project-specific binding tags to Gin's
// validator engine (e.g. `binding:"tw_phone"`). Must run before any request
// is bound, so NewRouter calls it during setup.
func registerCustomValidators() {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		return
	}
	_ = v.RegisterValidation("tw_phone", func(fl validator.FieldLevel) bool {
		return twPhonePattern.MatchString(fl.Field().String())
	})
}
