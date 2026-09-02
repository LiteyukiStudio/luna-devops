package identityapi

import (
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func configBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func requestUsesBearerToken(ctx *gin.Context) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(ctx.GetHeader("Authorization"))), "bearer ")
}

func lockActiveUserRole(tx *gorm.DB, userID, requiredRole string) (model.User, error) {
	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ? and disabled = ?", userID, false).Error; err != nil {
		return model.User{}, err
	}
	if requiredRole != "" && user.Role != requiredRole {
		return model.User{}, gorm.ErrRecordNotFound
	}
	return user, nil
}
