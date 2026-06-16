package classification

// CREDIT TO REPOSITORY: https://github.com/jbrukh/bayesian/blob/master/bayesian.go

import (
	"errors"
	"sync"
)

// defaultProb is the tiny non-zero probability that a word
// we have not seen before appears in the class. This is used
// as a fallback when Laplace smoothing cannot be applied
// (e.g., when the classifier has no training data).
const defaultProb = 1e-11

var ErrUnderflow = errors.New("possible underflow detected")
var ErrClassExists = errors.New("class already exists")
var ErrAlreadyConverted = errors.New("cannot add class after TF-IDF conversion")

type Class string

// Classifier implements the Naive Bayesian Classifier.
type Classifier struct {
	Classes         []Class
	learned         int   // docs learned
	seen            int32 // docs seen
	datas           map[Class]*classData
	tfIdf           bool
	DidConvertTfIdf bool         // we cannot classify a TF-IDF classifier if we haven't yet called ConvertTermsFreqToTfIdf
	mu              sync.RWMutex // protects Classes and datas for concurrent access
}

// serializableClassifier represents a container for
// Classifier objects whose fields are modifiable by
// reflection and are therefore writeable by gob.

type serializableClassifier struct {
	Classes         []Class
	Learned         int
	Seen            int
	Datas           map[Class]*classData
	TfIdf           bool
	DidConvertTfidf bool
}

// classData holds the frequency data for words in a
// particular class. In the future, we may replace this
// structure with a trie-like structure for more
// efficient storage.
type classData struct {
	Freqs   map[string]float64
	FreqTfs map[string][]float64
	Total   int
}

// newClassData creates a new empty classData node.
func newClassData() *classData {
	return &classData{
		Freqs:   make(map[string]float64),
		FreqTfs: make(map[string][]float64),
	}
}

// getWordProb returns P(W|C_j) -- the probability of seeing
// a particular word W in a document of this class.
// Uses Laplace smoothing (add-one smoothing) to handle unseen words:
// P(W|C) = (count(W,C) + 1) / (total_words_in_C + vocabulary_size)
func (d *classData) getWordProb(word string) float64 {
	vocab := len(d.Freqs)
	if d.Total == 0 || vocab == 0 {
		return defaultProb
	}

	value := d.Freqs[word] // 0 if not found
	return (value + 1) / (float64(d.Total) + float64(vocab))
}
