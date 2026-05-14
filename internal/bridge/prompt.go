package bridge

import ai "github.com/theopenbee/openbee/internal/ai"

func BuildBeeSessionPrefix() string {
	return ai.BuildBeeSessionPrefix()
}

func BuildWorkerSessionPrefix(persona WorkerPersona) string {
	return ai.BuildWorkerSessionPrefix(ai.WorkerPersona(persona.Name, persona.Description, persona.Constraints))
}
