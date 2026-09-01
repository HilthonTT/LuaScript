package classification

import (
	"math"
	"sync"
	"sync/atomic"
)

const defaultProb = 1e-11

type Class string

type Classifier struct {
	Classes         []Class
	learned         int
	seen            int32
	datas           map[Class]*classData
	tfIdf           bool
	DidConvertTfIdf bool
	mu              sync.RWMutex
}

type classData struct {
	Freqs   map[string]float64
	FreqTfs map[string][]float64
	Total   int
}

func newClassData() *classData {
	return &classData{
		Freqs:   make(map[string]float64),
		FreqTfs: make(map[string][]float64),
	}
}

func (d *classData) getWordProb(word string) float64 {
	vocab := len(d.Freqs)
	if d.Total == 0 || vocab == 0 {
		return defaultProb
	}

	value := d.Freqs[word]
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

func NewClassifierTfIdf(classes ...Class) *Classifier {
	return newClassifier(true, classes)
}

func NewClassifier(classes ...Class) *Classifier {
	return newClassifier(false, classes)
}

func (c *Classifier) getPriors() []float64 {
	n := len(c.Classes)
	priors := make([]float64, n)
	sum := 0

	for i, class := range c.Classes {
		total := c.datas[class].Total
		priors[i] = float64(total)
		sum += total
	}

	floatN := float64(n)
	floatSum := float64(sum)

	for i := range n {
		priors[i] = (priors[i] + 1) / (floatSum + floatN)
	}

	return priors
}

func (c *Classifier) Learned() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.learned
}

func (c *Classifier) Learn(document []string, which Class) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.tfIdf {
		if c.DidConvertTfIdf {
			panicIfAlreadyConverted()
		}

		docTf := make(map[string]float64)
		for _, word := range document {
			docTf[word]++
		}

		docLen := float64(len(document))

		for wIndex, wCount := range docTf {
			docTf[wIndex] = wCount / docLen
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

func (c *Classifier) ConvertTermsFreqToTfIdf() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.DidConvertTfIdf {
		panicIfAlreadyConverted()
	}

	logLearned := math.Log1p(float64(c.learned))

	for _, data := range c.datas {
		if data.Total == 0 {
			continue
		}
		invTotal := 1.0 / float64(data.Total)

		for wIndex, samples := range data.FreqTfs {
			tfIdfAdder := float64(0)

			for i, tf := range samples {
				tfIdf := math.Log1p(tf) * logLearned * invTotal
				samples[i] = tfIdf
				tfIdfAdder += tfIdf
			}
			data.Freqs[wIndex] = tfIdfAdder
		}
	}

	c.DidConvertTfIdf = true
}

func (c *Classifier) LogScores(document []string) ([]float64, int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.tfIdf && !c.DidConvertTfIdf {
		panic("Using a TF-IDF classifier. Please call ConvertTermsFreqToTfIdf before calling LogScores.")
	}

	n := len(c.Classes)
	scores := make([]float64, n)
	priors := c.getPriors()

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

func (c *Classifier) Classify(document []string) (Class, []float64, bool) {
	scores, inx, strict := c.LogScores(document)
	class := c.Classes[inx]
	return class, scores, strict
}

func (c *Classifier) ProbScores(doc []string) ([]float64, int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.tfIdf && !c.DidConvertTfIdf {
		panic("Using a TF-IDF classifier. Please call ConvertTermsFreqToTfIdf before calling ProbScores.")
	}

	n := len(c.Classes)
	scores := make([]float64, n)
	priors := c.getPriors()
	sum := float64(0)
	strict := false
	inx := 0

	for i, class := range c.Classes {
		data := c.datas[class]
		score := priors[i]
		for _, word := range doc {
			score *= data.getWordProb(word)
		}
		scores[i] = score
		sum += score
	}
	if sum == 0 {
		equal := 1.0 / float64(n)
		for i := range n {
			scores[i] = equal
		}
		strict = false
	} else {
		for i := range n {
			scores[i] /= sum
		}
		inx, strict = findMax(scores)
	}
	atomic.AddInt32(&c.seen, 1)
	return scores, inx, strict
}

func (c *Classifier) ClassifyProb(document []string) (Class, []float64, bool) {
	scores, inx, strict := c.ProbScores(document)
	class := c.Classes[inx]
	return class, scores, strict
}

func panicIfAlreadyConverted() {
	panic("Cannot call ConvertTermsFreqToTfIdf more than once. Reset and relearn to reconvert.")
}

func findMax(scores []float64) (int, bool) {
	inx := 0
	strict := true

	for i := 1; i < len(scores); i++ {
		if scores[inx] < scores[i] {
			inx = i
			strict = true
		} else if scores[inx] == scores[i] {
			strict = false
		}
	}

	return inx, strict
}
