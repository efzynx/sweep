package docker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

const (
	ModuleID       = "docker"
	ModuleName     = "Docker Engine"
	ModuleCategory = "containers"
	ModuleDesc     = "Clean stopped containers, dangling images, unused volumes, and build cache"
)

// CommandRunner defines the function signature for running external CLI commands.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Module implements core.Module for the Docker container engine.
type Module struct {
	commandRunner CommandRunner
	daemonChecker func(ctx context.Context) bool
}

// New creates and returns a new Docker module.
func New() *Module {
	m := &Module{
		commandRunner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
	}
	m.daemonChecker = func(ctx context.Context) bool {
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, err := m.commandRunner(checkCtx, "docker", "info", "--format", "{{.ServerVersion}}")
		return err == nil
	}
	return m
}

func init() {
	_ = core.Register(New())
}

func (m *Module) ID() string {
	return ModuleID
}

func (m *Module) Name() string {
	return ModuleName
}

func (m *Module) Category() string {
	return ModuleCategory
}

func (m *Module) Description() string {
	return ModuleDesc
}

func (m *Module) IsAvailable() bool {
	if !distro.HasDocker() {
		return false
	}
	if m.daemonChecker != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return m.daemonChecker(ctx)
	}
	return true
}

func (m *Module) RequiresElevation() bool {
	return false
}

// Scan queries Docker for stopped containers, dangling images, unused volumes, and build cache.
func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: time.Now(),
	}

	if !m.IsAvailable() {
		return result, nil
	}

	// 1. Stopped containers
	containers, err := m.scanStoppedContainers(ctx)
	if err == nil {
		result.Items = append(result.Items, containers...)
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 2. Dangling images
	images, err := m.scanDanglingImages(ctx)
	if err == nil {
		result.Items = append(result.Items, images...)
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 3. Unused volumes
	volumes, err := m.scanUnusedVolumes(ctx)
	if err == nil {
		result.Items = append(result.Items, volumes...)
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 4. Build cache
	buildCache, err := m.scanBuildCache(ctx)
	if err == nil && buildCache != nil {
		result.Items = append(result.Items, *buildCache)
	}

	result.RecalculateTotal()
	return result, nil
}

func (m *Module) scanStoppedContainers(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	out, err := m.commandRunner(ctx, "docker", "ps", "-a", "-f", "status=exited", "--format", "{{.ID}}|{{.Names}}|{{.Size}}")
	if err != nil {
		return items, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 2 {
			continue
		}

		containerID := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		sizeStr := ""
		if len(parts) >= 3 {
			sizeStr = strings.TrimSpace(parts[2])
		}

		sizeBytes := parseDockerSize(sizeStr)

		items = append(items, core.CleanableItem{
			ID:          "docker-container:" + containerID,
			Name:        fmt.Sprintf("Stopped container: %s (%s)", name, containerID),
			Description: fmt.Sprintf("Exited Docker container %s", name),
			Size:        sizeBytes,
			Selected:    true,
			Safety:      core.SafetySafe,
			Metadata: map[string]string{
				"type":         "container",
				"container_id": containerID,
				"name":         name,
			},
		})
	}

	return items, nil
}

func (m *Module) scanDanglingImages(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	out, err := m.commandRunner(ctx, "docker", "images", "-f", "dangling=true", "--format", "{{.ID}}|{{.Repository}}:{{.Tag}}|{{.Size}}")
	if err != nil {
		return items, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 2 {
			continue
		}

		imageID := strings.TrimSpace(parts[0])
		repoTag := strings.TrimSpace(parts[1])
		sizeStr := ""
		if len(parts) >= 3 {
			sizeStr = strings.TrimSpace(parts[2])
		}

		sizeBytes := parseDockerSize(sizeStr)

		items = append(items, core.CleanableItem{
			ID:          "docker-image:" + imageID,
			Name:        fmt.Sprintf("Dangling image: %s", imageID),
			Description: fmt.Sprintf("Untagged dangling image (%s)", repoTag),
			Size:        sizeBytes,
			Selected:    true,
			Safety:      core.SafetySafe,
			Metadata: map[string]string{
				"type":     "image",
				"image_id": imageID,
				"repo_tag": repoTag,
			},
		})
	}

	return items, nil
}

func (m *Module) scanUnusedVolumes(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	out, err := m.commandRunner(ctx, "docker", "volume", "ls", "-q", "-f", "dangling=true")
	if err != nil {
		return items, err
	}

	var volumeNames []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			volumeNames = append(volumeNames, name)
		}
	}

	if len(volumeNames) == 0 {
		return items, nil
	}

	// Try to get volume sizes via docker system df -v
	volumeSizes := make(map[string]int64)
	if dfOut, dfErr := m.commandRunner(ctx, "docker", "system", "df", "-v"); dfErr == nil {
		volumeSizes = parseDockerSystemDFVolumes(string(dfOut))
	}

	for _, name := range volumeNames {
		size := volumeSizes[name]
		items = append(items, core.CleanableItem{
			ID:          "docker-volume:" + name,
			Name:        fmt.Sprintf("Unused volume: %s", name),
			Description: fmt.Sprintf("Dangling volume not connected to any container (%s)", name),
			Size:        size,
			Selected:    false, // Caution: user data volumes default to unselected
			Safety:      core.SafetyCaution,
			Metadata: map[string]string{
				"type":        "volume",
				"volume_name": name,
			},
		})
	}

	return items, nil
}

func (m *Module) scanBuildCache(ctx context.Context) (*core.CleanableItem, error) {
	out, err := m.commandRunner(ctx, "docker", "system", "df", "--format", "{{.Type}}|{{.Size}}|{{.Reclaimable}}")
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Build Cache|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				reclaimableRaw := strings.TrimSpace(parts[2])
				// Format is usually "500MB (100%)" or "0B"
				sizeBytes := parseDockerSize(reclaimableRaw)
				if sizeBytes > 0 {
					return &core.CleanableItem{
						ID:          "docker-build-cache",
						Name:        "Docker Build Cache",
						Description: "Reclaimable BuildKit/builder cache layers",
						Size:        sizeBytes,
						Selected:    true,
						Safety:      core.SafetySafe,
						Metadata: map[string]string{
							"type": "build_cache",
						},
					}, nil
				}
			}
		}
	}

	return nil, nil
}

// Clean deletes the selected Docker items respecting DryRun.
func (m *Module) Clean(ctx context.Context, items []core.CleanableItem, opts core.CleanOptions) (core.CleanResult, error) {
	start := time.Now()
	cleanRes := core.CleanResult{
		CleanedItems: make([]core.CleanableItem, 0),
		Errors:       make([]error, 0),
	}

	if len(items) == 0 {
		cleanRes.Duration = time.Since(start)
		return cleanRes, nil
	}

	if opts.DryRun {
		for _, item := range items {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
			cleanRes.FreedBytes += item.Size
		}
		cleanRes.Duration = time.Since(start)
		return cleanRes, nil
	}

	var containers []string
	var images []string
	var volumes []string
	cleanBuildCache := false
	var buildCacheItem core.CleanableItem

	itemMap := make(map[string]core.CleanableItem)

	for _, item := range items {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		itemType := item.Metadata["type"]
		switch itemType {
		case "container":
			id := item.Metadata["container_id"]
			if id == "" {
				id = strings.TrimPrefix(item.ID, "docker-container:")
			}
			containers = append(containers, id)
			itemMap["c:"+id] = item

		case "image":
			id := item.Metadata["image_id"]
			if id == "" {
				id = strings.TrimPrefix(item.ID, "docker-image:")
			}
			images = append(images, id)
			itemMap["i:"+id] = item

		case "volume":
			name := item.Metadata["volume_name"]
			if name == "" {
				name = strings.TrimPrefix(item.ID, "docker-volume:")
			}
			volumes = append(volumes, name)
			itemMap["v:"+name] = item

		case "build_cache":
			cleanBuildCache = true
			buildCacheItem = item
		}
	}

	// 1. Delete containers: docker rm <ids...>
	if len(containers) > 0 {
		for _, cid := range containers {
			_, err := m.commandRunner(ctx, "docker", "rm", cid)
			it := itemMap["c:"+cid]
			if err != nil {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to rm container %s: %w", cid, err))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, it)
				cleanRes.FreedBytes += it.Size
			}
		}
	}

	// 2. Delete images: docker rmi <ids...>
	if len(images) > 0 {
		for _, imgID := range images {
			_, err := m.commandRunner(ctx, "docker", "rmi", imgID)
			it := itemMap["i:"+imgID]
			if err != nil {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to rmi image %s: %w", imgID, err))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, it)
				cleanRes.FreedBytes += it.Size
			}
		}
	}

	// 3. Delete volumes: docker volume rm <names...>
	if len(volumes) > 0 {
		for _, vName := range volumes {
			_, err := m.commandRunner(ctx, "docker", "volume", "rm", vName)
			it := itemMap["v:"+vName]
			if err != nil {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to rm volume %s: %w", vName, err))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, it)
				cleanRes.FreedBytes += it.Size
			}
		}
	}

	// 4. Delete build cache: docker builder prune -f
	if cleanBuildCache {
		_, err := m.commandRunner(ctx, "docker", "builder", "prune", "-f")
		if err != nil {
			// Fallback to buildx prune
			_, err = m.commandRunner(ctx, "docker", "buildx", "prune", "-f")
		}
		if err != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to prune build cache: %w", err))
		} else {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, buildCacheItem)
			cleanRes.FreedBytes += buildCacheItem.Size
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

// SetCommandRunner overrides command runner for testing.
func (m *Module) SetCommandRunner(runner CommandRunner) {
	m.commandRunner = runner
}

// SetDaemonChecker overrides daemon health checker for testing.
func (m *Module) SetDaemonChecker(fn func(ctx context.Context) bool) {
	m.daemonChecker = fn
}

// parseDockerSize parses size strings from Docker CLI like "12.5MB", "150kB", "12.3MB (virtual 150MB)", "10GB (100%)".
func parseDockerSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Split by space to ignore extra metadata like "(virtual ...)" or "(100%)"
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0
	}

	cleanStr := parts[0]
	cleanStr = strings.ReplaceAll(cleanStr, "\u00a0", " ")

	bytes, err := core.ParseBytes(cleanStr)
	if err != nil {
		return 0
	}
	return bytes
}

// parseDockerSystemDFVolumes extracts volume names and sizes from `docker system df -v` output.
func parseDockerSystemDFVolumes(output string) map[string]int64 {
	sizes := make(map[string]int64)
	scanner := bufio.NewScanner(strings.NewReader(output))

	inVolumesSection := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Local Volumes space usage:") {
			inVolumesSection = true
			continue
		}

		if inVolumesSection {
			if strings.HasPrefix(line, "Build cache usage:") || (strings.HasSuffix(line, "space usage:") && !strings.HasPrefix(line, "Local Volumes")) {
				break
			}
			if strings.HasPrefix(line, "VOLUME NAME") || line == "" {
				continue
			}

			fields := strings.Fields(line)
			// Format: <VOLUME NAME> <LINKS> <SIZE>
			if len(fields) >= 3 {
				vName := fields[0]
				vSizeStr := fields[len(fields)-1]
				sizes[vName] = parseDockerSize(vSizeStr)
			}
		}
	}

	return sizes
}
