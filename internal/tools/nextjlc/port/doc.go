// 本包是 upstream nextjlc（Rust）的逐模块 Go 直译。
//
// 对照关系：
//
//	src/file_type.rs   → file_type.go
//	src/rename.rs      → rename.go
//	src/header.rs      → header.go
//	src/outline.rs     → outline.go
//	src/validation.rs  → validation.go
//	src/fingerprint.rs → fingerprint.go
//	src/dcode.rs       → dcode.go
//	src/drill.rs       → drill.go
//
// 直译原则：规则表、匹配顺序、输出文件名、返回值语义都与原实现一致，
// 不自行"优化"命名或顺序 —— 这些细节直接决定 JLC 能不能正确识别层。
package port
