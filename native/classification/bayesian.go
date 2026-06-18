package classification

// CREDIT TO REPOSITORY: https://github.com/jbrukh/bayesian/blob/master/bayesian.go

import (
	"encoding/gob"
	"errors"
	"io"
	"math"
	"os"
	"sync"
	"sync/atomic"
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

func newClassifier(tfIdf bool, classes []Class) *Classifier {
	n := len(classes)
	if n < 2 {
		panic("provide at least two classes")
	}

	check := make(map[Class]struct{}, n)
	for _, class := range classes {
		check[class] = struct{}{}
	}

	if len(check) != n {
		panic("classes must be unique")
	}

	c := &Classifier{
		Classes: classes,
		datas:   make(map[Class]*classData, n),
		tfIdf:   tfIdf,
	}

	for _, class := range classes {
		c.datas[class] = newClassData()
	}

	return c
}

// NewClassifierTfIdf returns a new TF-IDF classifier. The classes provided
// should be at least 2 in number and unique, or this method will panic.
func NewClassifierTfIdf(classes ...Class) *Classifier {
	return newClassifier(true, classes)
}

// NewClassifier returns a new classifier. The classes provided
// should be at least 2 in number and unique, or this method will panic.
func NewClassifier(classes ...Class) *Classifier {
	return newClassifier(false, classes)
}

// NewClassifierFromFile loads an existing classifier from
// file. The classifier was previously saved with a call
// to c.WriteToFile(string).
func NewClassifierFromFile(name string) (*Classifier, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return NewClassifierFromReader(file)
}

// NewClassifierFromReader: This actually does the deserializing of a Gob encoded classifier
func NewClassifierFromReader(r io.Reader) (*Classifier, error) {
	dec := gob.NewDecoder(r)
	w := new(serializableClassifier)
	err := dec.Decode(w)

	return &Classifier{
		Classes:         w.Classes,
		learned:         w.Learned,
		seen:            int32(w.Seen),
		datas:           w.Datas,
		tfIdf:           w.TfIdf,
		DidConvertTfIdf: w.DidConvertTfidf,
	}, err
}

// AddClass adds a new class to the classifier dynamically.
// Returns ErrClassExists if the class already exists, or
// ErrAlreadyConverted if the classifier has been converted to TF-IDF.
// This method is safe for concurrent use.
func (c *Classifier) AddClass(class Class) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if the TF-IDF conversion already happened
	if c.DidConvertTfIdf {
		return ErrAlreadyConverted
	}

	if _, exists := c.datas[class]; exists {
		return ErrClassExists
	}

	c.Classes = append(c.Classes, class)
	c.datas[class] = newClassData()
	return nil
}

// getPriors returns the prior probabilities for the
// classes provided -- P(C_j).
// Uses Laplace smoothing to ensure no prior is zero:
// P(C_j) = (count_j + 1) / (total + num_classes)
func (c *Classifier) getPriors() []float64 {
	n := len(c.Classes)
	priors := make([]float64, n)
	sum := 0

	for i, class := range c.Classes {
		total := c.datas[class].Total
		priors[i] = float64(total)
		sum += total
	}

	// Apply Laplace smoothing to priors to avoid log(0)
	floatN := float64(n)
	floatSum := float64(sum)

	for i := range n {
		priors[i] = (priors[i] + 1) / (floatSum + floatN)
	}

	return priors
}

// Learned returns the number of documents ever learned
// in the lifetime of this classifier.
func (c *Classifier) Learned() int {
	return c.learned
}

// Seen returns the number of documents ever classified
// in the lifetime of this classifier.
func (c *Classifier) Seen() int {
	return int(atomic.LoadInt32(&c.seen))
}

// IsTfIdf returns true if we are a classifier of type TfIdf
func (c *Classifier) IsTfIdf() bool {
	return c.tfIdf
}

// WordCount returns the number of words counted for
// each class in the lifetime of the classifier.
// This method is safe for concurrent use.
func (c *Classifier) WordCount() []int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]int, len(c.Classes))
	for i, class := range c.Classes {
		data := c.datas[class]
		result[i] = data.Total
	}

	return result
}

// Observe should be used when word-frequencies have been already been learned
// externally (e.g., hadoop).
// This method is safe for concurrent use.
func (c *Classifier) Observe(word string, count int, which Class) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data := c.datas[which]
	data.Freqs[word] += float64(count)
	data.Total += count
}

// Learn will accept new training documents for
// supervised learning.
// This method is safe for concurrent use.
func (c *Classifier) Learn(document []string, which Class) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If we are a tfidf classifier we first need to get terms as
	// terms frequency and store that to work out the idf part later
	// in ConvertToIDF().
	if c.tfIdf {
		if c.DidConvertTfIdf {
			panicIfAlreadyConverted()
		}

		// Term Frequency: word count in document / document length
		docTf := make(map[string]float64)
		for _, word := range document {
			docTf[word]++
		}

		docLen := float64(len(document))

		for wIndex, wCount := range docTf {
			docTf[wIndex] = wCount / docLen
			// add the TF sample, after training we can get IDF values.
			c.datas[which].FreqTfs[wIndex] = append(c.datas[which].FreqTfs[wIndex], docTf[wIndex])
		}
	}

	data := c.datas[which]
	for _, word := range document {
		data.Freqs[word]++
		data.Total++
	}
	c.learned++
}

// ConvertTermsFreqToTfIdf uses all the TF samples for the class and converts
// them to TF-IDF https://en.wikipedia.org/wiki/Tf%E2%80%93idf
// once we have finished learning all the classes and have the totals.
// This method is safe for concurrent use.
func (c *Classifier) ConvertTermsFreqToTfIdf() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.DidConvertTfIdf {
		panicIfAlreadyConverted()
	}

	for className := range c.datas {
		for wIndex := range c.datas[className].FreqTfs {
			tfIdfAdder := float64(0)

			for tfSampleIndex := range c.datas[className].FreqTfs[wIndex] {
				// we always want a positive TF-IDF score.
				tf := c.datas[className].FreqTfs[wIndex][tfSampleIndex]
				c.datas[className].FreqTfs[wIndex][tfSampleIndex] = math.Log1p(tf) * math.Log1p(float64(c.learned)) / float64(c.datas[className].Total)
				tfIdfAdder += c.datas[className].FreqTfs[wIndex][tfSampleIndex]
			}
			// convert the 'counts' to TF-IDF's
			c.datas[className].Freqs[wIndex] = tfIdfAdder
		}
	}

	c.DidConvertTfIdf = true
}

// LogScores produces "log-likelihood"-like scores that can
// be used to classify documents into classes.
//
// The value of the score is proportional to the likelihood,
// as determined by the classifier, that the given document
// belongs to the given class. This is true even when scores
// returned are negative, which they will be (since we are
// taking logs of probabilities).
//
// The index j of the score corresponds to the class given
// by c.Classes[j].
//
// Additionally returned are "inx" and "strict" values. The
// inx corresponds to the maximum score in the array. If more
// than one of the scores holds the maximum values, then
// strict is false.
//
// Unlike c.Probabilities(), this function is not prone to
// floating point underflow and is relatively safe to use.
// This method is safe for concurrent use.
func (c *Classifier) LogScores(document []string) ([]float64, int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.tfIdf && !c.DidConvertTfIdf {
		panic("Using a TF-IDF classifier. Please call ConvertTermsFreqToTfIdf before calling LogScores.")
	}

	n := len(c.Classes)
	scores := make([]float64, n)
	priors := c.getPriors()

	// Calculate the score for each class
	for i, class := range c.Classes {
		data := c.datas[class]
		score := math.Log(priors[i])
		for _, word := range document {
			score += math.Log(data.getWordProb(word))
		}
		scores[i] = score
	}

	inx, strict := findMax(scores)
	atomic.AddInt32(&c.seen, 1)
	return scores, inx, strict
}

// Classify returns the most likely class for the given document
// along with the log scores and whether the classification is strict.
// This is a convenience wrapper around LogScores that returns the
// Class directly instead of an index.
func (c *Classifier) Classify(document []string) (Class, []float64, bool) {
	scores, ixn, strict := c.LogScores(document)
	class := c.Classes[ixn]
	return class, scores, strict
}

func panicIfAlreadyConverted() {
	panic("Cannot call ConvertTermsFreqToTfIdf more than once. Reset and relearn to reconvert.")
}

// findMax finds the maximum of a set of scores; if the
// maximum is strict -- that is, it is the single unique
// maximum from the set -- then strict has return value
// true. Otherwise it is false.
func findMax(scores []float64) (int, bool) {
	ixn := 0
	strict := true

	for i := 1; i < len(scores); i++ {
		if scores[ixn] < scores[i] {
			ixn = i
			strict = true
		} else if scores[ixn] == scores[i] {
			strict = false
		}
	}

	return ixn, strict
}
