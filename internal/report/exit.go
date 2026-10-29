package report

// ExitCode 把结果映射为设计文档 §10.2 的退出码：
// 0 成功；1 校验失败；3 schema 版本不支持（2 保留给用法/IO 错误）。
func ExitCode(res Result) int {
	for _, e := range res.Errors {
		if e.Code == CodeSchemaVersion {
			return 3
		}
	}
	if !res.OK {
		return 1
	}
	return 0
}
