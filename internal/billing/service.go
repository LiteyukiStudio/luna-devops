package billing

import (
	"errors"

	"gorm.io/gorm"
)

var ErrAlreadySettled = errors.New("billing usage already settled")

type Service struct {
	DB *gorm.DB
}
