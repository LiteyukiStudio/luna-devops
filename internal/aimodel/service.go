package aimodel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/fixeddecimal"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// aiModelCatalogMutationLockID serializes catalog mutations across API replicas.
// The value is the ASCII representation of "LUNAIMOD" and must remain stable.
const aiModelCatalogMutationLockID int64 = 0x4c554e41494d4f44

var (
	ErrDatabaseUnavailable      = errors.New("ai model database is unavailable")
	ErrLastEnabled              = errors.New("at least one AI model must remain enabled")
	ErrNameConflict             = errors.New("AI model name is already in use")
	ErrNotFound                 = errors.New("AI model was not found")
	ErrNameRequired             = errors.New("AI model name is required")
	ErrInputPriceInvalid        = errors.New("AI model input price is invalid")
	ErrOutputPriceInvalid       = errors.New("AI model output price is invalid")
	ErrCachedInputPriceInvalid  = errors.New("AI model cached input price is invalid")
	ErrCachedOutputPriceInvalid = errors.New("AI model cached output price is invalid")
	ErrMaxContextTokensInvalid  = errors.New("AI model context token limit is invalid")
	ErrMaxOutputTokensInvalid   = errors.New("AI model output token limit is invalid")
)

const (
	MinModelContextTokens int64 = 4_096
	MaxModelContextTokens int64 = 2_097_152
	MinModelOutputTokens  int64 = 256
	MaxModelOutputTokens  int64 = 262_144
)

type WriteInput struct {
	Name                          string `json:"name"`
	MaxContextTokens              int64  `json:"maxContextTokens"`
	MaxOutputTokens               int64  `json:"maxOutputTokens"`
	InputCreditsPerMillion        string `json:"inputCreditsPerMillion"`
	OutputCreditsPerMillion       string `json:"outputCreditsPerMillion"`
	CachedInputCreditsPerMillion  string `json:"cachedInputCreditsPerMillion"`
	CachedOutputCreditsPerMillion string `json:"cachedOutputCreditsPerMillion"`
	Enabled                       *bool  `json:"enabled"`
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) ListEnabled(ctx context.Context) ([]model.AIModel, error) {
	if s == nil || s.db == nil {
		return nil, ErrDatabaseUnavailable
	}
	var models []model.AIModel
	err := s.db.WithContext(ctx).Where("enabled = ?", true).Order("name asc").Find(&models).Error
	return models, err
}

func (s *Service) ListAll(ctx context.Context) ([]model.AIModel, error) {
	if s == nil || s.db == nil {
		return nil, ErrDatabaseUnavailable
	}
	var models []model.AIModel
	err := s.db.WithContext(ctx).Order("name asc").Find(&models).Error
	return models, err
}

func (s *Service) Create(ctx context.Context, input WriteInput) (created model.AIModel, err error) {
	ctx, end := telemetry.StartOperation(ctx, "ai_model", "create")
	defer func() {
		if err != nil {
			code, outcome := mutationFailure(err)
			telemetry.RecordError(ctx, "ai.model.create_failed", err,
				slog.String("operation", "create"),
				slog.String("outcome", outcome),
				slog.String("error.code", code),
				slog.String("resource.type", "ai_model"),
			)
		}
		end(err)
	}()

	created, err = parseInput(input, true)
	if err != nil {
		return model.AIModel{}, err
	}
	if s == nil || s.db == nil {
		return model.AIModel{}, ErrDatabaseUnavailable
	}
	created.ID = id.New("aimod")
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if lockErr := lockCatalogMutations(tx); lockErr != nil {
			return lockErr
		}
		var enabledCount int64
		if countErr := tx.Model(&model.AIModel{}).Where("enabled = ?", true).Count(&enabledCount).Error; countErr != nil {
			return countErr
		}
		if enabledCount == 0 {
			created.Enabled = true
		}
		// Enabled=false is an intentional catalog state, so persist every field
		// selected by the service instead of relying on database defaults.
		if createErr := tx.Select("*").Create(&created).Error; createErr != nil {
			return normalizePersistenceError(createErr)
		}
		return nil
	})
	if err != nil {
		return model.AIModel{}, err
	}
	telemetry.Logger().InfoContext(ctx, "AI model created",
		slog.String("event.name", "ai.model.created"),
		slog.String("operation", "create"),
		slog.String("outcome", "succeeded"),
		slog.String("resource.type", "ai_model"),
		slog.String("resource.id", created.ID),
		slog.Bool("ai.model.enabled", created.Enabled),
	)
	return created, nil
}

func (s *Service) Update(ctx context.Context, modelID string, input WriteInput) (updated model.AIModel, err error) {
	ctx, end := telemetry.StartOperation(ctx, "ai_model", "update")
	defer func() {
		if err != nil {
			code, outcome := mutationFailure(err)
			telemetry.RecordError(ctx, "ai.model.update_failed", err,
				slog.String("operation", "update"),
				slog.String("outcome", outcome),
				slog.String("error.code", code),
				slog.String("resource.type", "ai_model"),
				slog.String("resource.id", modelID),
			)
		}
		end(err)
	}()

	modelID = strings.TrimSpace(modelID)
	parsed, err := parseInput(input, false)
	if err != nil {
		return model.AIModel{}, err
	}
	if modelID == "" {
		return model.AIModel{}, ErrNotFound
	}
	if s == nil || s.db == nil {
		return model.AIModel{}, ErrDatabaseUnavailable
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if lockErr := lockCatalogMutations(tx); lockErr != nil {
			return lockErr
		}
		var current model.AIModel
		if findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", modelID).Error; findErr != nil {
			if errors.Is(findErr, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return findErr
		}
		applyInput(&current, parsed, input)
		desiredEnabled := current.Enabled
		if input.Enabled != nil {
			desiredEnabled = *input.Enabled
		}
		if !desiredEnabled {
			var enabledCount int64
			if countErr := tx.Model(&model.AIModel{}).
				Where("enabled = ? AND id <> ?", true, current.ID).
				Count(&enabledCount).Error; countErr != nil {
				return countErr
			}
			if enabledCount == 0 {
				if current.Enabled {
					return ErrLastEnabled
				}
				// Repair catalogs left without an enabled model by an older race.
				desiredEnabled = true
			}
		}
		current.Enabled = desiredEnabled
		if saveErr := tx.Save(&current).Error; saveErr != nil {
			return normalizePersistenceError(saveErr)
		}
		updated = current
		return nil
	})
	if err != nil {
		return model.AIModel{}, err
	}
	telemetry.Logger().InfoContext(ctx, "AI model updated",
		slog.String("event.name", "ai.model.updated"),
		slog.String("operation", "update"),
		slog.String("outcome", "succeeded"),
		slog.String("resource.type", "ai_model"),
		slog.String("resource.id", updated.ID),
		slog.Bool("ai.model.enabled", updated.Enabled),
	)
	return updated, nil
}

func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrNameRequired):
		return "ai.model_name_required"
	case errors.Is(err, ErrInputPriceInvalid):
		return "ai.model_input_price_invalid"
	case errors.Is(err, ErrOutputPriceInvalid):
		return "ai.model_output_price_invalid"
	case errors.Is(err, ErrCachedInputPriceInvalid):
		return "ai.model_cached_input_price_invalid"
	case errors.Is(err, ErrCachedOutputPriceInvalid):
		return "ai.model_cached_output_price_invalid"
	case errors.Is(err, ErrMaxContextTokensInvalid):
		return "ai.model_context_limit_invalid"
	case errors.Is(err, ErrMaxOutputTokensInvalid):
		return "ai.model_output_limit_invalid"
	case errors.Is(err, ErrNameConflict):
		return "ai.model_name_conflict"
	case errors.Is(err, ErrLastEnabled):
		return "ai.last_model_cannot_be_disabled"
	case errors.Is(err, ErrNotFound):
		return "ai.model_not_found"
	default:
		return ""
	}
}

func parseInput(input WriteInput, create bool) (model.AIModel, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return model.AIModel{}, ErrNameRequired
	}
	if input.MaxContextTokens < MinModelContextTokens || input.MaxContextTokens > MaxModelContextTokens {
		return model.AIModel{}, ErrMaxContextTokensInvalid
	}
	if input.MaxOutputTokens < MinModelOutputTokens || input.MaxOutputTokens > MaxModelOutputTokens || input.MaxOutputTokens >= input.MaxContextTokens {
		return model.AIModel{}, ErrMaxOutputTokensInvalid
	}
	result := model.AIModel{Name: name, MaxContextTokens: input.MaxContextTokens, MaxOutputTokens: input.MaxOutputTokens, Enabled: true}
	for _, field := range []struct {
		raw string
		out *decimal.Decimal
		err error
	}{
		{input.InputCreditsPerMillion, &result.InputCreditsPerMillion, ErrInputPriceInvalid},
		{input.OutputCreditsPerMillion, &result.OutputCreditsPerMillion, ErrOutputPriceInvalid},
		{input.CachedInputCreditsPerMillion, &result.CachedInputCreditsPerMillion, ErrCachedInputPriceInvalid},
		{input.CachedOutputCreditsPerMillion, &result.CachedOutputCreditsPerMillion, ErrCachedOutputPriceInvalid},
	} {
		if strings.TrimSpace(field.raw) == "" {
			if create {
				return model.AIModel{}, field.err
			}
			continue
		}
		value, parseErr := fixeddecimal.Parse(field.raw, true, fixeddecimal.Numeric24Scale8Max)
		if parseErr != nil {
			return model.AIModel{}, field.err
		}
		*field.out = value
	}
	if input.Enabled != nil {
		result.Enabled = *input.Enabled
	}
	return result, nil
}

func applyInput(current *model.AIModel, parsed model.AIModel, input WriteInput) {
	current.Name = parsed.Name
	current.MaxContextTokens = parsed.MaxContextTokens
	current.MaxOutputTokens = parsed.MaxOutputTokens
	if strings.TrimSpace(input.InputCreditsPerMillion) != "" {
		current.InputCreditsPerMillion = parsed.InputCreditsPerMillion
	}
	if strings.TrimSpace(input.OutputCreditsPerMillion) != "" {
		current.OutputCreditsPerMillion = parsed.OutputCreditsPerMillion
	}
	if strings.TrimSpace(input.CachedInputCreditsPerMillion) != "" {
		current.CachedInputCreditsPerMillion = parsed.CachedInputCreditsPerMillion
	}
	if strings.TrimSpace(input.CachedOutputCreditsPerMillion) != "" {
		current.CachedOutputCreditsPerMillion = parsed.CachedOutputCreditsPerMillion
	}
}

func lockCatalogMutations(tx *gorm.DB) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", aiModelCatalogMutationLockID).Error
}

func normalizePersistenceError(err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "ai_models_name_key") ||
		(strings.Contains(message, "duplicate key") && strings.Contains(message, "ai_models")) {
		return fmt.Errorf("%w: %v", ErrNameConflict, err)
	}
	return err
}

func mutationFailure(err error) (code, outcome string) {
	code = ErrorCode(err)
	if code == "" {
		return "ai.model_write_failed", "failed"
	}
	return code, "rejected"
}
