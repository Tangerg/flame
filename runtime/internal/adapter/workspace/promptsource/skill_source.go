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

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
	"github.com/Tangerg/flame/packages/go-sdk/fileinput"
	"github.com/Tangerg/flame/packages/go-sdk/pathidentity"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainskills "github.com/Tangerg/flame/runtime/internal/domain/skills"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/skilldocument"
)

// SkillSource is Runtime's finite admission boundary around the Agent
// Skills SDK. The SDK remains the format/resource implementation; Runtime owns
// the complete-list, document, and model-resource contract required by its
// model and UI consumers.
type SkillSource struct {
	root      string
	resources sdk.ResourceSource
}

// NewSkillSource admits documents and resources beneath an explicit physical
// boundary. Discovery, execution and automatic revision share this admission.
func NewSkillSource(root, boundary string) (*SkillSource, error) {
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
	return &SkillSource{root: physicalRoot, resources: repository}, nil
}

func (r *SkillSource) List(ctx context.Context) ([]sdk.Summary, error) {
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

func (r *SkillSource) loadSummaries(ctx context.Context, names []string) ([]sdk.Summary, error) {
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

func (r *SkillSource) loadSummary(ctx context.Context, name string) (sdk.Summary, bool, error) {
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

func (r *SkillSource) Lookup(ctx context.Context, name string) (sdk.Summary, error) {
	return r.resources.Lookup(ctx, name)
}

func (r *SkillSource) Load(ctx context.Context, name string) (*sdk.Skill, error) {
	skill, _, err := r.document(ctx, name)
	return skill, err
}

func (r *SkillSource) document(ctx context.Context, name string) (*sdk.Skill, []byte, error) {
	if !validRuntimeSkillName(name) {
		return nil, nil, fmt.Errorf("%w %q: invalid name", sdk.ErrInvalidSkill, name)
	}
	if err := skillSourceContextError(ctx, "load"); err != nil {
		return nil, nil, err
	}
	source, err := r.openSkillDocument(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			_, lookupErr := r.Lookup(ctx, name)
			return nil, nil, errors.Join(err, lookupErr)
		}
		return nil, nil, err
	}
	content, err := readSkillDocument(ctx, name, source)
	if cancelErr := skillSourceContextError(ctx, "load"); cancelErr != nil {
		return nil, nil, errors.Join(err, cancelErr)
	}
	if err != nil {
		return nil, nil, err
	}
	skill, err := skilldocument.Load(ctx, name, content)
	return skill, content, err
}

type openedSkillDocument struct {
	root *os.Root
	file *os.File
	info os.FileInfo
	path string
}

func (o *openedSkillDocument) close() error {
	return errors.Join(o.file.Close(), o.root.Close())
}

func (r *SkillSource) openSkillDocument(name string) (*openedSkillDocument, error) {
	root, err := os.OpenRoot(r.root)
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: open %q: %w", r.root, err)
	}
	path := filepath.Join(name, sdk.SkillFile)
	file, info, openErr := fileinput.OpenAt(root, path, domainskills.MaxAuthoredSkillDocumentBytes)
	if openErr != nil {
		if closeErr := root.Close(); closeErr != nil {
			return nil, fmt.Errorf("runtime skill source: open %q: %w", name, errors.Join(openErr, closeErr))
		}
		switch {
		case errors.Is(openErr, fileinput.ErrNotRegular):
			return nil, fmt.Errorf("%w %q: document admission: %w", sdk.ErrInvalidSkill, name, openErr)
		case errors.Is(openErr, fileinput.ErrTooLarge):
			return nil, fmt.Errorf(
				"%w: %q document admission: %w",
				domainskills.ErrDocumentTooLarge,
				name,
				openErr,
			)
		default:
			return nil, fmt.Errorf("runtime skill source: open %q: %w", name, openErr)
		}
	}
	return &openedSkillDocument{root: root, file: file, info: info, path: path}, nil
}

func readSkillDocument(ctx context.Context, name string, source *openedSkillDocument) ([]byte, error) {
	content, readErr := io.ReadAll(io.LimitReader(
		cancellation.Reader(ctx, source.file),
		domainskills.MaxAuthoredSkillDocumentBytes+1,
	))
	verifyErr := fileinput.VerifyAtVersion(source.file, source.info, source.root, source.path)
	closeErr := source.close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf(
			"runtime skill source: read %q: %w",
			name,
			errors.Join(readErr, verifyErr, closeErr),
		)
	}
	if len(content) > domainskills.MaxAuthoredSkillDocumentBytes {
		return nil, fmt.Errorf(
			"%w: %q exceeds %d bytes",
			domainskills.ErrDocumentTooLarge,
			name,
			domainskills.MaxAuthoredSkillDocumentBytes,
		)
	}
	if verifyErr != nil {
		return nil, fmt.Errorf("runtime skill source: read %q: %w", name, verifyErr)
	}
	return content, nil
}

func (r *SkillSource) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
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

func (r *SkillSource) directoryEntries(ctx context.Context) ([]fs.DirEntry, error) {
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
	if err := ctx.Err(); err != nil {
		cause := context.Cause(ctx)
		if !errors.Is(cause, err) {
			cause = errors.Join(err, cause)
		}
		return fmt.Errorf("runtime skill source: %s: %w", operation, cause)
	}
	return nil
}

type boundedSkillResource struct {
	fs.File
	ctx      context.Context
	name     string
	resource string
	read     int64
}

func (b *boundedSkillResource) Read(buffer []byte) (int, error) {
	if err := skillSourceContextError(b.ctx, "read resource"); err != nil {
		return 0, err
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
	b.read += int64(read)
	if int64(read) > remaining {
		read = int(remaining)
		err = errors.Join(err, b.tooLarge())
	}
	return read, errors.Join(err, skillSourceContextError(b.ctx, "read resource"))
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

func (r *SkillSource) documentPath(name string) string {
	return filepath.Join(r.root, name, sdk.SkillFile)
}
