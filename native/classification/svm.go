package classification

import "math"

// CODE FROM REPO: https://github.com/datastream/libsvm

type SVMType int

const (
	/* SVM Type */
	CSVC SVMType = iota
	NUSVC
	ONECLASS
	EPSILONSVR
	NUSVR
)

type KernelType int

const (
	/* KERNEL Type */
	LINEAR KernelType = iota
	POLY
	RBF
	SIGMOID
	PRECOMPUTED
)

type SVMNode struct {
	Index int
	Value float64
}

func (s *SVMNode) clone() *SVMNode {
	return &SVMNode{
		Index: s.Index,
		Value: s.Value,
	}
}

// SVMParameter define param for svm
type SVMParameter struct {
	SvmType    SVMType
	KernelType KernelType
	Degree     int     // for poly
	Gamma      float64 // for poly/rbf/sigmoid
	Coef0      float64

	// these are for training only
	CacheSize   float64
	Eps         float64
	C           float64
	NrWeight    int
	WeightLabel []int
	Weight      []float64
	Nu          float64
	P           float64
	Shrinking   int
	Probability int
}

// Clone SVMParameter
func (s *SVMParameter) Clone() *SVMParameter {
	rst := &SVMParameter{
		SvmType:     s.SvmType,
		KernelType:  s.KernelType,
		Degree:      s.Degree,
		Gamma:       s.Gamma,
		Coef0:       s.Coef0,
		CacheSize:   s.CacheSize,
		Eps:         s.Eps,
		NrWeight:    s.NrWeight,
		Nu:          s.Nu,
		P:           s.P,
		Shrinking:   s.Shrinking,
		Probability: s.Probability,
	}
	copy(rst.WeightLabel, s.WeightLabel)
	copy(rst.Weight, s.Weight)
	return rst
}

// SVMModel define model of svm
type SVMModel struct {
	Param     *SVMParameter // parameter
	NrClass   int           // number of classes, = 2 in regression/one class svm
	L         int           // total #SV
	SV        [][]SVMNode   // SVs (SV[l])
	SvCoef    [][]float64   // coefficients for SVs in decision functions (svCoef[k-1][l])
	Rho       []float64     // constants in decision functions (rho[k*(k-1)/2])
	ProbA     []float64     // pariwise probability information
	ProbB     []float64
	SvIndices []int // svIndices[0,...,nSV-1] are values in [1,...,numTraningData] to indicate SVs in the training set

	// for classification only
	Label []int // label of each class (label[k])
	NSV   []int // number of SVs for each class (nSV[k])  nSV[0] + nSV[1] + ... + nSV[k-1] = l
}

// SVMProblem define svm problem
type SVMProblem struct {
	L int
	Y []float64
	X [][]SVMNode
}

type svmHeadT struct {
	prev, next *svmHeadT
	data       []float32
	length     int
}

type SVMCache struct {
	l       int
	size    int64
	head    []*svmHeadT
	lruHead *svmHeadT
}

func NewSVMCache(l int, size int64) *SVMCache {
	c := &SVMCache{
		l:    l,
		size: size,
		head: make([]*svmHeadT, l),
	}

	for i := range l {
		c.head[i] = &svmHeadT{}
	}

	c.size /= 4
	c.size -= int64(l * (16 / 4))                             // sizeof(svmHeadT) == 16
	c.size = int64(math.Max(float64(c.size), float64(2*c.l))) // cache must be large enough for two columns
	c.lruHead = &svmHeadT{}
	c.lruHead.prev = c.lruHead
	c.lruHead.next = c.lruHead.prev

	return c
}

func (*SVMCache) lruDelete(h *svmHeadT) {
	// delete from current location
	h.prev.next = h.next
	h.next.prev = h.prev
}

func (c *SVMCache) lruInsert(h *svmHeadT) {
	h.next = c.lruHead
	h.prev = c.lruHead.prev
	h.prev.next = h
	h.next.prev = h
}

// request data [0,length)
// return some position p where [p,length) need to be filled
// (p >= length if nothing needs to be filled)
// java: simulate pointer using single-element array

func (m *SVMCache) getData(index int, data [][]float32, length int) int {
	h := m.head[index]
	if h.length > 0 {
		m.lruDelete(h)
	}
	more := length - h.length
	if more > 0 {
		// free old space
		for m.size < int64(more) {
			old := m.lruHead.next
			m.lruDelete(old)
			m.size += int64(old.length)
			old.data = nil
			old.length = 0
		}

		// allocate new space
		newData := make([]float32, length)
		if h.data != nil {
			// System.arraycopy(h.data,0,newData,0,h.length)
			copy(newData, h.data[:length])
		}
		h.data = newData
		m.size -= int64(more)
		// do {int _=h.length; h.length=length; length=_;} while(false);
		h.length, length = length, h.length
	}

	m.lruInsert(h)
	data[0] = h.data
	return length
}
