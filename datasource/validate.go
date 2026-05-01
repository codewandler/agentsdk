package datasource

import "fmt"

// Validate checks datasource definition invariants that are independent of any
// app registry or action resolver.
func Validate(def Definition) error {
	if def.Name == "" {
		return fmt.Errorf("datasource: name is required")
	}
	if def.Kind == "" {
		return fmt.Errorf("datasource %q: kind is required", def.Name)
	}
	if err := validateCredentials(def); err != nil {
		return err
	}
	if err := validateActionRefs(def); err != nil {
		return err
	}
	return nil
}

func validateCredentials(def Definition) error {
	seen := map[string]bool{}
	for _, cred := range def.Credentials {
		if cred.Name == "" {
			return fmt.Errorf("datasource %q: credential name is required", def.Name)
		}
		if seen[cred.Name] {
			return fmt.Errorf("datasource %q: duplicate credential %q", def.Name, cred.Name)
		}
		seen[cred.Name] = true
	}
	return nil
}

func validateActionRefs(def Definition) error {
	for op, ref := range map[string]string{
		"fetch":     def.Actions.Fetch.Name,
		"list":      def.Actions.List.Name,
		"search":    def.Actions.Search.Name,
		"sync":      def.Actions.Sync.Name,
		"map":       def.Actions.Map.Name,
		"transform": def.Actions.Transform.Name,
	} {
		if ref == "" {
			continue
		}
		if ref == def.Name {
			return fmt.Errorf("datasource %q: %s action ref must not point at datasource name", def.Name, op)
		}
	}
	return nil
}
