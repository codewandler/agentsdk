package browserplugin

import (
	"context"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
)

const maxElements = 80

// interactiveRoles are the accessibility roles we include in the element tree.
var interactiveRoles = map[string]bool{
	"link":       true,
	"button":     true,
	"textbox":    true,
	"searchbox":  true,
	"combobox":   true,
	"checkbox":   true,
	"radio":      true,
	"menuitem":   true,
	"tab":        true,
	"switch":     true,
	"slider":     true,
	"spinbutton": true,
	"option":     true,
}

// structuralRoles are included for context but not as interactable targets.
var structuralRoles = map[string]bool{
	"heading":    true,
	"navigation": true,
	"main":       true,
	"form":       true,
	"region":     true,
	"banner":     true,
	"contentinfo": true,
}

// Element represents a single interactable or structural element.
type Element struct {
	Index    int
	Role     string
	Name     string
	Selector string
	Children []*Element
}

// extractElements fetches the accessibility tree and returns a filtered list
// of interactive elements with resolved CSS selectors.
func extractElements(ctx context.Context) ([]*Element, error) {
	// Enable accessibility and get the full tree.
	var nodes []*accessibility.Node
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		nodes, err = accessibility.GetFullAXTree().Do(ctx)
		return err
	}))
	if err != nil {
		return nil, fmt.Errorf("get accessibility tree: %w", err)
	}

	// Filter to interactive + structural nodes.
	var elements []*Element
	idx := 0
	for _, node := range nodes {
		if node.Ignored {
			continue
		}
		role := axValueString(node.Role)
		name := axValueString(node.Name)

		isInteractive := interactiveRoles[role]
		isStructural := structuralRoles[role]

		if !isInteractive && !isStructural {
			continue
		}
		if isInteractive && name == "" {
			continue // skip unnamed interactive elements
		}

		idx++
		elem := &Element{
			Role: role,
			Name: name,
		}
		if isInteractive {
			elem.Index = idx
		}

		// Resolve CSS selector from backend DOM node ID.
		if node.BackendDOMNodeID != 0 {
			sel := resolveSelector(ctx, node.BackendDOMNodeID)
			if sel != "" {
				elem.Selector = sel
			}
		}

		elements = append(elements, elem)
		if idx >= maxElements {
			break
		}
	}

	return elements, nil
}

// resolveSelector attempts to build a minimal CSS selector for a DOM node.
func resolveSelector(ctx context.Context, backendID cdp.BackendNodeID) string {
	var desc *cdp.Node
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		desc, err = dom.DescribeNode().WithBackendNodeID(backendID).Do(ctx)
		return err
	}))
	if err != nil || desc == nil {
		return ""
	}

	// Strategy: #id > [aria-label] > tag.class > tag
	nodeName := strings.ToLower(desc.NodeName)

	// Check for ID attribute.
	for i := 0; i+1 < len(desc.Attributes); i += 2 {
		if desc.Attributes[i] == "id" && desc.Attributes[i+1] != "" {
			return fmt.Sprintf("%s#%s", nodeName, desc.Attributes[i+1])
		}
	}

	// Check for aria-label.
	for i := 0; i+1 < len(desc.Attributes); i += 2 {
		if desc.Attributes[i] == "aria-label" && desc.Attributes[i+1] != "" {
			return fmt.Sprintf("%s[aria-label=%q]", nodeName, desc.Attributes[i+1])
		}
	}

	// Check for href on links.
	if nodeName == "a" {
		for i := 0; i+1 < len(desc.Attributes); i += 2 {
			if desc.Attributes[i] == "href" && desc.Attributes[i+1] != "" {
				return fmt.Sprintf("a[href=%q]", desc.Attributes[i+1])
			}
		}
	}

	// Fallback: tag + class.
	for i := 0; i+1 < len(desc.Attributes); i += 2 {
		if desc.Attributes[i] == "class" && desc.Attributes[i+1] != "" {
			classes := strings.Fields(desc.Attributes[i+1])
			if len(classes) > 0 {
				return nodeName + "." + classes[0]
			}
		}
	}

	return nodeName
}

// renderElementTree produces the compact text representation for the context provider.
func renderElementTree(elements []*Element, sessionID, url, title string, total int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[browser: %s | %s | %q]\n", sessionID, url, title)

	for _, elem := range elements {
		if elem.Index > 0 {
			// Interactive element with index.
			if elem.Selector != "" {
				fmt.Fprintf(&sb, "  [%d] %s %q → %s\n", elem.Index, elem.Role, elem.Name, elem.Selector)
			} else {
				fmt.Fprintf(&sb, "  [%d] %s %q\n", elem.Index, elem.Role, elem.Name)
			}
		} else {
			// Structural element (heading, nav, etc).
			if elem.Name != "" {
				fmt.Fprintf(&sb, "\n%s %q\n", elem.Role, elem.Name)
			} else {
				fmt.Fprintf(&sb, "\n%s\n", elem.Role)
			}
		}
	}

	if total > maxElements {
		fmt.Fprintf(&sb, "\n(+%d more below fold)\n", total-maxElements)
	}

	return sb.String()
}

// axValueString extracts the string value from an accessibility Value.
func axValueString(v *accessibility.Value) string {
	if v == nil {
		return ""
	}
	// Value.Value is a jsontext.Value (raw JSON). Try to unquote it.
	raw := string(v.Value)
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return raw[1 : len(raw)-1]
	}
	return raw
}
