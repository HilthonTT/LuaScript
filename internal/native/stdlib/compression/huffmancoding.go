package compression

import (
	"fmt"
	"sort"
)

type Node struct {
	left   *Node
	right  *Node
	symbol rune
	weight int
}

type SymbolFreq struct {
	Symbol rune
	Freq   int
}

func HuffTree(listfreq []SymbolFreq) (*Node, error) {
	if len(listfreq) < 1 {
		return nil, fmt.Errorf("huffman coding: HuffTree : calling method with empty list of symbol-frequency pairs")
	}

	q1 := make([]Node, len(listfreq))
	q2 := make([]Node, 0, len(listfreq))
	for i, x := range listfreq {
		q1[i] = Node{left: nil, right: nil, symbol: x.Symbol, weight: x.Freq}
	}

	for len(q1)+len(q2) > 1 {
		var node1, node2 Node
		node1, q1, q2 = least(q1, q2)
		node2, q1, q2 = least(q1, q2)
		node := Node{left: &node1, right: &node2,
			symbol: -1, weight: node1.weight + node2.weight}
		q2 = append(q2, node)
	}
	if len(q1) == 1 {
		return &q1[0], nil
	}
	return &q2[0], nil
}

func least(q1 []Node, q2 []Node) (Node, []Node, []Node) {
	if len(q1) == 0 {
		return q2[0], q1, q2[1:]
	}
	if len(q2) == 0 {
		return q1[0], q1[1:], q2
	}
	if q1[0].weight <= q2[0].weight {
		return q1[0], q1[1:], q2
	}
	return q2[0], q1, q2[1:]
}

func HuffEncoding(node *Node, prefix []bool, codes map[rune][]bool) {
	if node.symbol != -1 {
		codes[node.symbol] = prefix
		return
	}

	prefixLeft := make([]bool, len(prefix))
	copy(prefixLeft, prefix)
	prefixLeft = append(prefixLeft, false)
	HuffEncoding(node.left, prefixLeft, codes)
	prefixRight := make([]bool, len(prefix))
	copy(prefixRight, prefix)
	prefixRight = append(prefixRight, true)
	HuffEncoding(node.right, prefixRight, codes)
}

func HuffEncode(codes map[rune][]bool, in string) []bool {
	out := make([]bool, 0)
	for i := 0; i < len(in); i++ {
		out = append(out, codes[rune(in[i])]...)
	}
	return out
}

func SymbolCountOrd(message string) []SymbolFreq {
	runeCount := make(map[rune]int)
	for i := 0; i < len(message); i++ {
		runeCount[rune(message[i])]++
	}
	listfreq := make([]SymbolFreq, len(runeCount))
	i := 0
	for s, n := range runeCount {
		listfreq[i] = SymbolFreq{Symbol: s, Freq: n}
		i++
	}
	sort.Slice(listfreq, func(i, j int) bool {
		return listfreq[i].Freq < listfreq[j].Freq
	})
	return listfreq
}
