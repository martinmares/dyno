package navigation

import (
	"fmt"
	"testing"
)

func TestDebugD2(t *testing.T) {
	nav, err := BuildTree("/Users/mares/Development/Src/Claude/dyno-site", "/docs")
	if err != nil {
		t.Fatal(err)
	}
	pages := FlatPages(nav)
	fmt.Println("All pages:")
	for _, p := range pages {
		fmt.Printf("  FullPath=%q FSPath=%q\n", p.FullPath, p.FSPath)
	}
	node := FindNode(nav, "/docs/guides/d2-diagrams")
	fmt.Printf("FindNode result: %v\n", node)
}
