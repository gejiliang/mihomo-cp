package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gejiliang/mihomo-cp/internal/model"
	"github.com/gejiliang/mihomo-cp/internal/store"
	"github.com/google/uuid"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// PublishService orchestrates the publish workflow.
type PublishService struct {
	publishStore *store.PublishStore
	configSvc    *ConfigService
	validator    *Validator
	mihomo       *MihomoClient
}

// NewPublishService creates a new PublishService.
func NewPublishService(ps *store.PublishStore, cs *ConfigService, v *Validator, mc *MihomoClient) *PublishService {
	return &PublishService{
		publishStore: ps,
		configSvc:    cs,
		validator:    v,
		mihomo:       mc,
	}
}

// PublishRequest holds all parameters needed to execute a publish.
type PublishRequest struct {
	ConfigYAML []byte
	ConfigPath string // path to write YAML
	ConfigDir  string // mihomo working directory
	MihomoBin  string // path to mihomo binary
	Operator   string
	Note       string
}

// ValidateWithMihomo writes the YAML to a temp dir, copies geodata files from
// the mihomo working dir, runs `mihomoBin -t -d tmpDir`, and returns the output.
func (s *PublishService) ValidateWithMihomo(yamlContent []byte, mihomoDir, mihomoBin string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "mihomo-validate-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, yamlContent, 0o644); err != nil {
		return "", fmt.Errorf("write temp config: %w", err)
	}

	// Copy geodata files from mihomo working dir to avoid re-downloading.
	geoFiles := []string{"GeoIP.dat", "geoip.dat", "GeoSite.dat", "geosite.dat", "country.mmdb", "country-lite.mmdb", "GeoLite2-ASN.mmdb"}
	for _, name := range geoFiles {
		src := filepath.Join(mihomoDir, name)
		if data, err := os.ReadFile(src); err == nil {
			_ = os.WriteFile(filepath.Join(tmpDir, name), data, 0o644)
		}
	}

	// Also copy providers directory if it exists.
	provSrc := filepath.Join(mihomoDir, "providers")
	if info, err := os.Stat(provSrc); err == nil && info.IsDir() {
		provDst := filepath.Join(tmpDir, "providers")
		_ = os.MkdirAll(provDst, 0o755)
		entries, _ := os.ReadDir(provSrc)
		for _, e := range entries {
			if !e.IsDir() {
				if data, err := os.ReadFile(filepath.Join(provSrc, e.Name())); err == nil {
					_ = os.WriteFile(filepath.Join(provDst, e.Name()), data, 0o644)
				}
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, mihomoBin, "-t", "-d", tmpDir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	cmdErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return out.String(), fmt.Errorf("mihomo validation timed out after 30s")
	}
	return out.String(), cmdErr
}

// generateDiff produces a human-readable unified-style diff between old and new YAML content.
func generateDiff(oldContent, newContent string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldContent, newContent, false)
	dmp.DiffCleanupSemantic(diffs)

	var sb strings.Builder
	for _, d := range diffs {
		lines := strings.Split(d.Text, "\n")
		for i, line := range lines {
			// Skip the trailing empty string from a trailing newline split
			if i == len(lines)-1 && line == "" {
				continue
			}
			switch d.Type {
			case diffmatchpatch.DiffInsert:
				sb.WriteString("+ ")
				sb.WriteString(line)
				sb.WriteString("\n")
			case diffmatchpatch.DiffDelete:
				sb.WriteString("- ")
				sb.WriteString(line)
				sb.WriteString("\n")
			case diffmatchpatch.DiffEqual:
				sb.WriteString("  ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

// Publish executes the full publish workflow:
// 1. Gets next version, creates a record, reads current config for diffing,
// 2. Writes new config, reloads mihomo,
// 3. Restores backup and re-reloads on failure.
func (s *PublishService) Publish(req PublishRequest) (*model.PublishRecord, error) {
	version, err := s.publishStore.NextVersion()
	if err != nil {
		return nil, fmt.Errorf("get next version: %w", err)
	}

	record := &model.PublishRecord{
		ID:         uuid.New().String(),
		Version:    version,
		ConfigYAML: string(req.ConfigYAML),
		Operator:   req.Operator,
		Note:       req.Note,
		CreatedAt:  time.Now().UTC(),
	}

	// Read current config for diff + backup
	var currentContent []byte
	currentContent, err = os.ReadFile(req.ConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read current config: %w", err)
	}

	// Generate diff
	record.DiffText = generateDiff(string(currentContent), string(req.ConfigYAML))

	// Write new config
	if err := os.WriteFile(req.ConfigPath, req.ConfigYAML, 0o644); err != nil {
		return nil, fmt.Errorf("write new config: %w", err)
	}

	// Reload mihomo — pass the config file path, not directory
	reloadErr := s.mihomo.ReloadConfig(req.ConfigPath)
	if reloadErr != nil {
		// Restore backup
		if len(currentContent) > 0 {
			_ = os.WriteFile(req.ConfigPath, currentContent, 0o644)
			_ = s.mihomo.ReloadConfig(req.ConfigDir)
		}
		record.Status = "failed"
		record.ErrorMsg = reloadErr.Error()
	} else {
		record.Status = "success"
	}

	if err := s.publishStore.Create(record); err != nil {
		return nil, fmt.Errorf("save publish record: %w", err)
	}

	return record, nil
}

// Rollback finds the last successful publish and re-publishes it.
func (s *PublishService) Rollback(configPath, configDir, operator string) (*model.PublishRecord, error) {
	last, err := s.publishStore.GetLastSuccess()
	if err != nil {
		return nil, fmt.Errorf("get last success: %w", err)
	}

	req := PublishRequest{
		ConfigYAML: []byte(last.ConfigYAML),
		ConfigPath: configPath,
		ConfigDir:  configDir,
		Operator:   operator,
		Note:       fmt.Sprintf("rollback to version %s", last.Version),
	}
	return s.Publish(req)
}
