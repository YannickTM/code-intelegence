package parser

// Batching limits for ParseFiles requests.
const (
	MaxFilesPerBatch = 50
	MaxBytesPerBatch = 5 * 1024 * 1024 // 5 MB
)

// FileInput represents a file to be parsed by the embedded parser engine.
type FileInput struct {
	FilePath string
	Content  string
	Language string
}

// BatchFileInputs splits files into batches that respect MaxFilesPerBatch
// and MaxBytesPerBatch. File ordering is preserved for deterministic behavior.
//
// A single file whose content exceeds MaxBytesPerBatch is placed in its own
// batch (it is never dropped).
func BatchFileInputs(files []FileInput) [][]FileInput {
	if len(files) == 0 {
		return nil
	}

	var batches [][]FileInput
	var current []FileInput
	var currentBytes int

	for _, f := range files {
		contentSize := len(f.Content)

		// If adding this file would exceed either limit, flush the current batch first.
		if len(current) > 0 && (len(current) >= MaxFilesPerBatch || currentBytes+contentSize > MaxBytesPerBatch) {
			batches = append(batches, current)
			current = nil
			currentBytes = 0
		}

		current = append(current, f)
		currentBytes += contentSize
	}

	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}
