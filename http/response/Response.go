package response

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ResponseResource struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Resource    interface{} `json:"resource,omitempty"`
}

func Success(message string, resource interface{}) ResponseResource {
	return ResponseResource{
		Status:  true,
		Message: message,
		Resource:  resource,
	}
}

func Error(message string) ResponseResource {
	return ResponseResource{
		Status:  false,
		Message: message,
	}
}

//====================== payload response
func FormatValidationError(err error) []map[string]string {
	var errors []map[string]string

	if ve, ok := err.(validator.ValidationErrors); ok {
		for _, e := range ve {
			field := formatField(e)
			message := formatMessage(e)

			errors = append(errors, map[string]string{
				"field":   field,
				"message": message,
			})
		}
	} else {
		errors = append(errors, map[string]string{
			"field":   "body",
			"message": err.Error(),
		})
	}

	return errors
}
func formatField(e validator.FieldError) string {
	field := e.Namespace() // contoh: payload.Products[0].Price

	field = strings.Replace(field, "payload.", "", 1)

	// ubah ke json style (snake_case kalau perlu manual)
	field = strings.ReplaceAll(field, ".", ".")

	// optional: ubah jadi lowercase
	field = strings.ToLower(field)

	return field
}
func formatMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", toSnakeCase(e.Field()))
	case "oneof":
		return fmt.Sprintf("%s must be one of %s", toSnakeCase(e.Field()), e.Param())
	case "min":
		return fmt.Sprintf("%s must be at least %s", toSnakeCase(e.Field()), e.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s", toSnakeCase(e.Field()), e.Param())
	default:
		return fmt.Sprintf("%s is invalid", toSnakeCase(e.Field()))
	}
}
func toSnakeCase(str string) string {
	var result []rune
	for i, r := range str {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}
//=====================================================