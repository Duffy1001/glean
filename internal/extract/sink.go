package extract

// ArraySink receives incremental root-array output for root-array extraction.
//
// Start/Finish must be called exactly once per extraction that uses the sink.
type ArraySink interface {
	Start() error
	WriteItem(item any) error
	Finish() error
}
