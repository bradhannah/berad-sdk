// Package authoring provides reusable, source-bound audio authoring services.
// It owns immutable recording analysis, schema-3 document/history and transactional
// publication, not project vocabulary, game routing semantics, terminal input or
// native device lifetime. The caller supplies the same Decoder used by playback
// and explicit calibration policy; analysis never opens a device.
//
// Sources eagerly retain at most 128 MiB of encoded data plus streamed 10 ms
// analysis buckets. Decoders receive private byte copies. This is an input limit,
// not a total-memory guarantee. Cancellation is cooperative between filesystem
// reads and analysis buckets; an in-flight codec/filesystem call must return.
// Editors are caller-serialized. Publication receipts survive failures and stale
// operation results; source verification is not a lock against external writers.
package authoring
