// Package text provides immutable font resources, styled layout and local text
// geometry and bounded Ebitengine rendering. Hosts own retained output, input,
// focus, scene ordering, destinations, and application text semantics.
//
// Font and content values are immutable. Layout queries do not service input or
// advance clocks. Renderers are host-serialized, explicitly closed resources;
// their caches never expose images. Caller-owned drawing targets survive Close.
package text
