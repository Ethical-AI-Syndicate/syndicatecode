package patch

func EngineRepoRoot(engine *Engine) string {
	if engine == nil {
		return ""
	}
	return engine.repoRoot
}
