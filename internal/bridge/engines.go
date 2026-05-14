package bridge

import ai "github.com/theopenbee/openbee/internal/ai"

type EngineSet struct {
	adapters map[string]ai.EngineAdapter
}

func NewEngineSet(adapters map[string]ai.EngineAdapter) EngineSet {
	return EngineSet{adapters: adapters}
}

func EngineSetForTest(adapters map[string]ai.EngineAdapter) EngineSet {
	return NewEngineSet(adapters)
}
