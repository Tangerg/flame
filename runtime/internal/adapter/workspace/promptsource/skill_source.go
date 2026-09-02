package promptsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	sdk "github.com/Tangerg/scope/skills"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/workspace/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

// runtimeSkillSource is Runtime's finite admission boundary around the Agent
// Skills SDK. The SDK remains the format/resource implementation; Runtime owns
// the complete-list, document, and model-resource contract required by its
// model and UI consumers.
type runtimeSkillSource struct {
	root      string
	resources sdk.ResourceSource
}

func newRuntimeSkillSource(root, boundary string) (*runtimeSkillSource, error) {
	physicalRoot, err := pathidentity.Resolve("", root)
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: resolve root %q: %w", root, err)
	}
	physicalBoundary, err := pathidentity.Resolve("", boundary)
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: resolve boundary %q: %w", boundary, err)
	}
	inside, err := pathidentity.Contains(physicalBoundary, physicalRoot)
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: confine root %q: %w", root, err)
	}
	if !inside {
		return nil, fmt.Errorf(
			"runtime skill source: root %q resolves outside %q: %w",
			root,
			boundary,
			workspaceapp.ErrPathOutsideRoot,
		)
	}
	repository, err := sdk.NewDirectoryRepository(physicalRoot, sdk.RepositoryConfig{})
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: open %q: %w", physicalRoot, err)
	}
	return &runtimeSkillSource{root: physicalRoot, resources: repository}, nil
}

func (r *runtimeSkillSource) List(ctx context.Context) ([]sdk.Summary, error) {
	entries, err := r.directoryEntries(ctx)
	if err != nil {
		return nil, err
	}
	return r.loadSummaries(ctx, skillCandidateNames(entries))
}

func skillCandidateNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, min(len(entries), domainskills.MaxSkillsPerSource))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !validRuntimeSkillName(name) {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (r *runtimeSkillSource) loadSummaries(ctx context.Context, names []string) ([]sdk.Summary, error) {
	summaries := make([]sdk.Summary, 0, len(names))
	for _, name := range names {
		summary, valid, err := r.loadSummary(ctx, name)
		if err != nil {
			return nil, err
		}
		if !valid {
			continue
		}
		if len(summaries) == domainskills.MaxSkillsPerSource {
			return nil, fmt.Errorf(
				"%w: source %q contains more than %d valid Skills",
				domainskills.ErrLibraryCapacity,
				r.root,
				domainskills.MaxSkillsPerSource,
			)
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func (r *runtimeSkillSource) loadSummary(ctx context.Context, name string) (sdk.Summary, bool, error) {
	if err := skillSourceContextError(ctx, "list"); err != nil {
		return sdk.Summary{}, false, err
	}
	skill, err := r.Load(ctx, name)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, sdk.ErrInvalidSkill) {
		return sdk.Summary{}, false, nil
	}
	if err != nil {
		return sdk.Summary{}, false, fmt.Errorf("runtime skill source: list: %w", err)
	}
	return skill.Summary(), true, nil
}

func (r *runtimeSkillSource) Load(ctx context.Context, name string) (*sdk.Skill, error) {
	if !validRuntimeSkillName(name) {
		return nil, fmt.Errorf("%w %q: invalid name", sdk.ErrInvalidSkill, name)
	}
	if err := skillSourceContextError(ctx, "load"); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(r.root)
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: open %q: %w", r.root, err)
	}
	defer func() { _ = root.Close() }()
	file, _, err := fileinput.OpenAt(root, filepath.Join(name, sdk.SkillFile), domainskills.MaxAuthoredSkillDocumentBytes)
	if err != nil {
		switch {
		case errors.Is(err, fileinput.ErrNotRegular):
			return nil, fmt.Errorf("runtime skill source: %q is not a regular document", name)
		case errors.Is(err, fileinput.ErrTooLarge):
			return nil, fmt.Errorf(
				"%w: %q exceeds %d bytes",
				domainskills.ErrDocumentTooLarge,
				name,
				domainskills.MaxAuthoredSkillDocumentBytes,
			)
		default:
			return nil, fmt.Errorf("runtime skill source: open %q: %w", name, err)
		}
	}
	content, readErr := io.ReadAll(io.LimitReader(
		skillSourceContextReader{ctx: ctx, reader: file},
		domainskills.MaxAuthoredSkillDocumentBytes+1,
	))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("runtime skill source: read %q: %w", name, errors.Join(readErr, closeErr))
	}
	if len(content) > domainskills.MaxAuthoredSkillDocumentBytes {
		return nil, fmt.Errorf(
			"%w: %q exceeds %d bytes",
			domainskills.ErrDocumentTooLarge,
			name,
			domainskills.MaxAuthoredSkillDocumentBytes,
		)
	}
	skill, err := sdk.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", sdk.ErrInvalidSkill, name, err)
	}
	if skill.Name != name {
		return nil, fmt.Errorf("%w %q: %w", sdk.ErrInvalidSkill, name, sdk.ErrNameMismatch)
	}
	return skill, nil
}

func (r *runtimeSkillSource) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	file, err := r.resources.OpenResource(ctx, name, resource)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("runtime skill source: inspect resource %q/%q: %w", name, resource, err), file.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(
			fmt.Errorf("runtime skill source: resource %q/%q is not a regular file", name, resource),
			file.Close(),
		)
	}
	if info.Size() > domainskills.MaxSkillResourceBytes {
		return nil, errors.Join(
			fmt.Errorf(
				"%w: resource %q/%q is %d bytes; limit is %d",
				domainskills.ErrResourceTooLarge,
				name,
				resource,
				info.Size(),
				domainskills.MaxSkillResourceBytes,
			),
			file.Close(),
		)
	}
	if err := skillSourceContextError(ctx, "open resource"); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return &boundedSkillResource{File: file, ctx: ctx, name: name, resource: resource}, nil
}

func (r *runtimeSkillSource) directoryEntries(ctx context.Context) ([]fs.DirEntry, error) {
	if err := skillSourceContextError(ctx, "list"); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(r.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: open %q: %w", r.root, err)
	}
	directory, _, err := fileinput.OpenDirectoryAt(root, ".")
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	entries, readErr := directory.ReadDir(domainskills.MaxSkillDirectoryEntries + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	closeErr := errors.Join(directory.Close(), root.Close())
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("runtime skill source: list %q: %w", r.root, errors.Join(readErr, closeErr))
	}
	if len(entries) > domainskills.MaxSkillDirectoryEntries {
		return nil, fmt.Errorf(
			"%w: source %q contains more than %d directory entries",
			domainskills.ErrLibraryCapacity,
			r.root,
			domainskills.MaxSkillDirectoryEntries,
		)
	}
	if err := skillSourceContextError(ctx, "list"); err != nil {
		return nil, err
	}
	return entries, nil
}

func validRuntimeSkillName(name string) bool {
	return sdk.ValidateName(name) == nil
}

func skillSourceContextError(ctx context.Context, operation string) error {
	if cause := context.Cause(ctx); cause != nil {
		return fmt.Errorf("runtime skill source: %s: %w", operation, cause)
	}
	return nil
}

type skillSourceContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (s skillSourceContextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(s.ctx); cause != nil {
		return 0, cause
	}
	read, err := s.reader.Read(buffer)
	if cause := context.Cause(s.ctx); cause != nil {
		return read, cause
	}
	return read, err
}

type boundedSkillResource struct {
	fs.File
	ctx      context.Context
	name     string
	resource string
	read     int64
}

func (b *boundedSkillResource) Read(buffer []byte) (int, error) {
	if cause := context.Cause(b.ctx); cause != nil {
		return 0, cause
	}
	remaining := int64(domainskills.MaxSkillResourceBytes) - b.read
	if remaining < 0 {
		return 0, b.tooLarge()
	}
	limit := int64(len(buffer))
	if limit > remaining+1 {
		limit = remaining + 1
	}
	read, err := b.File.Read(buffer[:limit])
	if cause := context.Cause(b.ctx); cause != nil {
		b.read += int64(read)
		return read, cause
	}
	if int64(read) <= remaining {
		b.read += int64(read)
		return read, err
	}
	allowed := int(remaining)
	b.read += int64(read)
	return allowed, b.tooLarge()
}

func (b *boundedSkillResource) tooLarge() error {
	return fmt.Errorf(
		"%w: resource %q/%q exceeds %d bytes",
		domainskills.ErrResourceTooLarge,
		b.name,
		b.resource,
		domainskills.MaxSkillResourceBytes,
	)
}
