// Package parser provides domain types for the parser subsystem.
package parser

// ParsedFileResult holds the analysis output for a single source file.
type ParsedFileResult struct {
	FilePath  string
	Language  string
	FileHash  string
	SizeBytes int64
	LineCount int32

	Symbols []Symbol
	Imports []Import
	Chunks  []Chunk
	Issues  []Issue

	// v2 extensions.
	Exports           []Export
	References        []Reference
	JsxUsages         []JsxUsage
	NetworkCalls      []NetworkCall
	Facts             *FileFacts
	Metadata          *ParserMeta
	ExtractorStatuses []ExtractorStatus
}

// Symbol represents a code symbol (function, class, variable, etc.).
type Symbol struct {
	SymbolID       string
	Name           string
	QualifiedName  string
	Kind           string
	Signature      string
	StartLine      int32
	EndLine        int32
	DocText        string
	SymbolHash     string
	ParentSymbolID string

	// v2 extensions.
	Flags          *SymbolFlags
	Modifiers      []string
	ReturnType     string
	ParameterTypes []string
}

// SymbolFlags holds boolean flags for a symbol.
type SymbolFlags struct {
	IsExported            bool
	IsDefaultExport       bool
	IsAsync               bool
	IsGenerator           bool
	IsStatic              bool
	IsAbstract            bool
	IsReadonly            bool
	IsOptional            bool
	IsArrowFunction       bool
	IsReactComponentLike  bool
	IsHookLike            bool
}

// Import represents a file-level import declaration.
type Import struct {
	SourceSymbolID string
	SourceFilePath string
	TargetFilePath string
	ImportName     string
	ImportType     string // INTERNAL | EXTERNAL | STDLIB
	PackageName    string
	PackageVersion string
}

// Chunk represents a code chunk suitable for embedding or retrieval.
type Chunk struct {
	ChunkID        string
	SymbolID       string
	ChunkType      string // function | class | module_context | config | test
	ChunkHash      string
	Content        string
	ContextBefore  string
	ContextAfter   string
	StartLine      int32
	EndLine        int32
	EstimatedTokens int32

	// v2 extensions.
	OwnerQualifiedName string
	OwnerKind          string
	IsExportedContext  bool
	SemanticRole       string // implementation | api_surface | config | test | ui_component | hook
}

// Issue represents a parser diagnostic (warning or informational).
type Issue struct {
	Code     string
	Message  string
	Line     int32
	Column   int32
	Severity string // info | warning | error
}

// Export represents a file-level export declaration.
type Export struct {
	ExportID     string
	FilePath     string
	ExportKind   string // NAMED | DEFAULT | REEXPORT | EXPORT_ALL | TYPE_ONLY
	ExportedName string
	LocalName    string
	SymbolID     string
	SourceModule string
	Line         int32
	Column       int32
}

// Reference represents an AST reference to another symbol.
type Reference struct {
	ReferenceID            string
	SourceFilePath         string
	SourceSymbolID         string
	ReferenceKind          string // CALL | JSX_RENDER | TYPE_REF | ...
	RawText                string
	TargetName             string
	QualifiedTargetHint    string
	StartLine              int32
	StartColumn            int32
	EndLine                int32
	EndColumn              int32
	ResolutionScope        string // LOCAL | IMPORTED | MEMBER | GLOBAL | UNKNOWN
	ResolvedTargetSymbolID string
	Confidence             string // HIGH | MEDIUM | LOW
}

// JsxUsage represents a JSX component usage.
type JsxUsage struct {
	UsageID                string
	SourceSymbolID         string
	ComponentName          string
	IsIntrinsic            bool
	IsFragment             bool
	Line                   int32
	Column                 int32
	ResolvedTargetSymbolID string
	Confidence             string
}

// NetworkCall represents a detected network/fetch call.
type NetworkCall struct {
	NetworkCallID  string
	SourceSymbolID string
	ClientKind     string // FETCH | AXIOS | KY | GRAPHQL | UNKNOWN
	Method         string // GET | POST | PUT | PATCH | DELETE | UNKNOWN
	URLLiteral     string
	URLTemplate    string
	IsRelative     bool
	StartLine      int32
	StartColumn    int32
	Confidence     string
}

// FileFacts holds boolean facts about a file's content.
type FileFacts struct {
	HasJsx                   bool
	HasDefaultExport         bool
	HasNamedExports          bool
	HasTopLevelSideEffects   bool
	HasReactHookCalls        bool
	HasFetchCalls            bool
	HasClassDeclarations     bool
	HasTests                 bool
	HasConfigPatterns        bool
	JsxRuntime               string // react | preact | unknown
}

// ParserMeta holds metadata about the parser that produced the result.
type ParserMeta struct {
	ParserVersion     string
	GrammarVersion    string
	EnabledExtractors []string
}

// ExtractorStatus reports the outcome of a single parser extractor.
type ExtractorStatus struct {
	ExtractorName string
	Status        string // OK | PARTIAL | FAILED
	IssueCount    int32
	Message       string
}

