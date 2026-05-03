package command

// ExportDescriptor is a machine-readable command descriptor projection for
// HTTP/OpenAPI-like channels. It keeps caller policy visible while preserving the
// original descriptor as the source of truth.
type ExportDescriptor struct {
	Descriptor   Descriptor `json:"descriptor"`
	Policy       Policy     `json:"policy"`
	InputSchema  JSONSchema `json:"inputSchema"`
	OutputSchema JSONSchema `json:"outputSchema,omitempty"`
}

// ExportDescriptors flattens executable descriptors into API-oriented command
// exports. Namespace-only nodes are omitted; subcommands are walked recursively.
func ExportDescriptors(descriptors []Descriptor) []ExportDescriptor {
	var out []ExportDescriptor
	for _, desc := range descriptors {
		appendExportDescriptors(&out, desc)
	}
	return out
}

func appendExportDescriptors(out *[]ExportDescriptor, desc Descriptor) {
	if desc.Executable {
		*out = append(*out, ExportDescriptor{
			Descriptor:   desc,
			Policy:       desc.Policy,
			InputSchema:  CommandInputSchema(desc),
			OutputSchema: desc.Output.Schema,
		})
	}
	for _, sub := range desc.Subcommands {
		appendExportDescriptors(out, sub)
	}
}
