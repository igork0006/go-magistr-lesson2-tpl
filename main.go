package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: yamlvalid <path_to_yaml>")
		os.Exit(1)
	}

	path := os.Args[1]
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot read file: %v\n", path, err)
		os.Exit(1)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		fmt.Fprintf(os.Stderr, "%s: cannot parse YAML: %v\n", path, err)
		os.Exit(1)
	}

	errorsFound := traversePod(path, &root)
	if len(errorsFound) > 0 {
		for _, e := range errorsFound {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}

	// Если ошибок нет
	os.Exit(0)
}

// ---------- Валидация Pod ----------

func traversePod(filename string, root *yaml.Node) []string {
	var errorsFound []string

	if len(root.Content) == 0 {
		errorsFound = append(errorsFound, fmt.Sprintf("%s: empty YAML document", filename))
		return errorsFound
	}

	doc := root.Content[0] // Берём первый документ
	m := nodeMap(doc)

	// apiVersion
	if n, ok := m["apiVersion"]; ok {
		if n.Value != "v1" {
			errorsFound = append(errorsFound, fmt.Sprintf("%s:%d apiVersion has unsupported value '%s'", filename, n.Line, n.Value))
		}
	} else {
		errorsFound = append(errorsFound, fmt.Sprintf("apiVersion is required"))
	}

	// kind
	if n, ok := m["kind"]; ok {
		if n.Value != "Pod" {
			errorsFound = append(errorsFound, fmt.Sprintf("%s:%d kind has unsupported value '%s'", filename, n.Line, n.Value))
		}
	} else {
		errorsFound = append(errorsFound, "kind is required")
	}

	// metadata
	metaNode, ok := m["metadata"]
	if !ok {
		errorsFound = append(errorsFound, "metadata is required")
	} else {
		errorsFound = append(errorsFound, traverseMetadata(filename, metaNode)...)
	}

	// spec
	specNode, ok := m["spec"]
	if !ok {
		errorsFound = append(errorsFound, "spec is required")
	} else {
		errorsFound = append(errorsFound, traverseSpec(filename, specNode)...)
	}

	return errorsFound
}

// ---------- Metadata ----------

func traverseMetadata(filename string, meta *yaml.Node) []string {
	var errorsFound []string
	m := nodeMap(meta)

	// name
	if n, ok := m["name"]; !ok || n.Value == "" {
		errorsFound = append(errorsFound, "metadata.name is required")
	}

	// namespace и labels необязательны, проверка типов не обязательна
	return errorsFound
}

// ---------- Spec ----------

func traverseSpec(filename string, spec *yaml.Node) []string {
	var errorsFound []string
	m := nodeMap(spec)

	// os
	if osNode, ok := m["os"]; ok {
		if osNode.Kind == yaml.ScalarNode {
			if osNode.Value != "linux" && osNode.Value != "windows" {
				errorsFound = append(errorsFound, fmt.Sprintf("%s:%d spec.os has unsupported value '%s'", filename, osNode.Line, osNode.Value))
			}
		}
	}

	// containers
	contNode, ok := m["containers"]
	if !ok {
		errorsFound = append(errorsFound, "spec.containers is required")
		return errorsFound
	}
	if contNode.Kind != yaml.SequenceNode {
		errorsFound = append(errorsFound, fmt.Sprintf("%s:%d spec.containers must be a list", filename, contNode.Line))
		return errorsFound
	}

	for _, c := range contNode.Content {
		errorsFound = append(errorsFound, traverseContainer(filename, c)...)
	}

	return errorsFound
}

// ---------- Container ----------

func traverseContainer(filename string, c *yaml.Node) []string {
	var errorsFound []string
	m := nodeMap(c)

	// name
	if n, ok := m["name"]; !ok || n.Value == "" {
		errorsFound = append(errorsFound, "containers.name is required")
	} else {
		matched, _ := regexp.MatchString(`^[a-z0-9_]+$`, n.Value)
		if !matched {
			errorsFound = append(errorsFound, fmt.Sprintf("%s:%d containers.name has invalid format '%s'", filename, n.Line, n.Value))
		}
	}

	// image
	if n, ok := m["image"]; !ok || n.Value == "" {
		errorsFound = append(errorsFound, "containers.image is required")
	} else {
		if !strings.HasPrefix(n.Value, "registry.bigbrother.io/") {
			errorsFound = append(errorsFound, fmt.Sprintf("%s:%d containers.image has unsupported value '%s'", filename, n.Line, n.Value))
		}
		if !strings.Contains(n.Value, ":") {
			errorsFound = append(errorsFound, fmt.Sprintf("%s:%d containers.image must include tag", filename, n.Line))
		}
	}

	// ports (необязательные)
	if portsNode, ok := m["ports"]; ok && portsNode.Kind == yaml.SequenceNode {
		for _, p := range portsNode.Content {
			errorsFound = append(errorsFound, traversePort(filename, p)...)
		}
	}

	// readinessProbe
	if rNode, ok := m["readinessProbe"]; ok {
		errorsFound = append(errorsFound, traverseProbe(filename, rNode, "readinessProbe")...)
	}

	// livenessProbe
	if lNode, ok := m["livenessProbe"]; ok {
		errorsFound = append(errorsFound, traverseProbe(filename, lNode, "livenessProbe")...)
	}

	// resources
	if resNode, ok := m["resources"]; ok {
		errorsFound = append(errorsFound, traverseResources(filename, resNode)...)
	} else {
		errorsFound = append(errorsFound, "containers.resources is required")
	}

	return errorsFound
}

// ---------- ContainerPort ----------

func traversePort(filename string, port *yaml.Node) []string {
	var errorsFound []string
	m := nodeMap(port)

	// containerPort
	if n, ok := m["containerPort"]; !ok {
		errorsFound = append(errorsFound, fmt.Sprintf("%s:%d containerPort is required", filename, port.Line))
	} else {
		if _, err := strconv.Atoi(n.Value); err != nil {
			errorsFound = append(errorsFound, fmt.Sprintf("%s:%d containerPort must be int", filename, n.Line))
		} else if v, _ := strconv.Atoi(n.Value); v <= 0 || v >= 65536 {
			errorsFound = append(errorsFound, fmt.Sprintf("%s:%d containerPort value out of range", filename, n.Line))
		}
	}

	// protocol
	if n, ok := m["protocol"]; ok {
		if n.Value != "TCP" && n.Value != "UDP" {
			errorsFound = append(errorsFound, fmt.Sprintf("%s:%d protocol has unsupported value '%s'", filename, n.Line, n.Value))
		}
	}

	return errorsFound
}

// ---------- Probe ----------

func traverseProbe(filename string, probe *yaml.Node, name string) []string {
	var errorsFound []string
	m := nodeMap(probe)
	httpNode, ok := m["httpGet"]
	if !ok {
		errorsFound = append(errorsFound, fmt.Sprintf("%s:%d %s.httpGet is required", filename, probe.Line, name))
		return errorsFound
	}
	m2 := nodeMap(httpNode)

	// path
	if n, ok := m2["path"]; !ok || n.Value == "" {
		errorsFound = append(errorsFound, fmt.Sprintf("%s:%d %s.httpGet.path is required", filename, httpNode.Line, name))
	} else if !strings.HasPrefix(n.Value, "/") {
		errorsFound = append(errorsFound, fmt.Sprintf("%s:%d %s.httpGet.path has invalid format '%s'", filename, n.Line, name, n.Value))
	}

	// port
	if n, ok := m2["port"]; !ok {
		errorsFound = append(errorsFound, fmt.Sprintf("%s:%d %s.httpGet.port is required", filename, httpNode.Line, name))
	} else if v, err := strconv.Atoi(n.Value); err != nil {
		errorsFound = append(errorsFound, fmt.Sprintf("%s:%d %s.httpGet.port must be int", filename, n.Line, name))
	} else if v <= 0 || v >= 65536 {
		errorsFound = append(errorsFound, fmt.Sprintf("%s:%d %s.httpGet.port value out of range", filename, n.Line, name))
	}

	return errorsFound
}

// ---------- Resources ----------

func traverseResources(filename string, res *yaml.Node) []string {
	var errorsFound []string
	m := nodeMap(res)

	checkMem := func(v string) bool {
		return regexp.MustCompile(`^\d+(Gi|Mi|Ki)$`).MatchString(v)
	}

	checkCPU := func(v string) bool {
		_, err := strconv.Atoi(v)
		return err == nil
	}

	for _, kind := range []string{"limits", "requests"} {
		if node, ok := m[kind]; ok {
			m2 := nodeMap(node)
			for k, n := range m2 {
				switch k {
				case "cpu":
					if !checkCPU(n.Value) {
						errorsFound = append(errorsFound, fmt.Sprintf("%s:%d %s.cpu must be int", filename, n.Line, kind))
					}
				case "memory":
					if !checkMem(n.Value) {
						errorsFound = append(errorsFound, fmt.Sprintf("%s:%d %s.memory has invalid format '%s'", filename, n.Line, kind, n.Value))
					}
				}
			}
		}
	}

	return errorsFound
}

// ---------- Утилита ----------

func nodeMap(n *yaml.Node) map[string]*yaml.Node {
	m := map[string]*yaml.Node{}
	if n.Kind == yaml.MappingNode {
		for i := 0; i < len(n.Content); i += 2 {
			keyNode := n.Content[i]
			valueNode := n.Content[i+1]
			m[keyNode.Value] = valueNode
		}
	}
	return m
}
