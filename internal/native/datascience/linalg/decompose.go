package linalg

import "math"

const tol = 1e-12

func cholesky(a [][]float64) ([][]float64, bool) {
	n := len(a)
	if !isSymmetric(a) {
		return nil, false
	}
	l := zeroMatrix(n, n)
	for i := range n {
		for j := 0; j <= i; j++ {
			s := 0.0
			for k := range j {
				s += l[i][k] * l[j][k]
			}
			if i == j {
				d := a[i][i] - s
				if d <= 0 {
					return nil, false
				}
				l[i][j] = math.Sqrt(d)
			} else {
				l[i][j] = (a[i][j] - s) / l[j][j]
			}
		}
	}
	return l, true
}

func qrDecompose(a [][]float64) (q, r [][]float64, ok bool) {
	m, n := len(a), cols(a)
	q = zeroMatrix(m, n)
	r = zeroMatrix(n, n)

	v := make([][]float64, n)
	for j := range n {
		v[j] = make([]float64, m)
		for i := range m {
			v[j][i] = a[i][j]
		}
	}

	for j := range n {
		nrm := 0.0
		for i := range m {
			nrm += v[j][i] * v[j][i]
		}
		nrm = math.Sqrt(nrm)
		if nrm < tol {
			return nil, nil, false
		}
		r[j][j] = nrm
		for i := range m {
			q[i][j] = v[j][i] / nrm
		}
		for k := j + 1; k < n; k++ {
			d := 0.0
			for i := range m {
				d += q[i][j] * v[k][i]
			}
			r[j][k] = d
			for i := range m {
				v[k][i] -= d * q[i][j]
			}
		}
	}
	return q, r, true
}

func lstsq(a [][]float64, b []float64) ([]float64, bool) {
	q, r, ok := qrDecompose(a)
	if !ok {
		return nil, false
	}
	m, n := len(a), cols(a)

	y := make([]float64, n)
	for j := range n {
		s := 0.0
		for i := range m {
			s += q[i][j] * b[i]
		}
		y[j] = s
	}

	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		s := y[i]
		for j := i + 1; j < n; j++ {
			s -= r[i][j] * x[j]
		}
		x[i] = s / r[i][i]
	}
	return x, true
}

func rank(a [][]float64) int {
	m := copyMatrix(a)
	rows, ncols := len(m), cols(a)
	if rows == 0 || ncols == 0 {
		return 0
	}
	r := 0
	for c := 0; c < ncols && r < rows; c++ {
		best, bestVal := r, math.Abs(m[r][c])
		for i := r + 1; i < rows; i++ {
			if v := math.Abs(m[i][c]); v > bestVal {
				best, bestVal = i, v
			}
		}
		if bestVal < tol {
			continue
		}
		m[r], m[best] = m[best], m[r]
		for i := r + 1; i < rows; i++ {
			f := m[i][c] / m[r][c]
			for j := c; j < ncols; j++ {
				m[i][j] -= f * m[r][j]
			}
		}
		r++
	}
	return r
}

func eigenSymmetric(a [][]float64) ([]float64, [][]float64) {
	n := len(a)
	m := copyMatrix(a)
	v := identityMatrix(n)

	const maxSweeps = 100
	for sweep := 0; sweep < maxSweeps; sweep++ {
		off := 0.0
		for i := range n {
			for j := i + 1; j < n; j++ {
				off += m[i][j] * m[i][j]
			}
		}
		if off < tol*tol {
			break
		}
		for p := range n {
			for q := p + 1; q < n; q++ {
				if math.Abs(m[p][q]) < tol {
					continue
				}
				theta := (m[q][q] - m[p][p]) / (2 * m[p][q])
				t := 1 / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				if theta < 0 {
					t = -t
				}
				c := 1 / math.Sqrt(t*t+1)
				s := t * c

				applyJacobiRotation(m, v, n, p, q, c, s)
			}
		}
	}

	values := make([]float64, n)
	for i := range n {
		values[i] = m[i][i]
	}
	sortEigenDescending(values, v)
	return values, v
}

func applyJacobiRotation(m, v [][]float64, n, p, q int, c, s float64) {
	mpp, mqq, mpq := m[p][p], m[q][q], m[p][q]
	m[p][p] = c*c*mpp - 2*s*c*mpq + s*s*mqq
	m[q][q] = s*s*mpp + 2*s*c*mpq + c*c*mqq
	m[p][q] = 0
	m[q][p] = 0

	for k := range n {
		if k == p || k == q {
			continue
		}
		mkp, mkq := m[k][p], m[k][q]
		m[k][p] = c*mkp - s*mkq
		m[p][k] = m[k][p]
		m[k][q] = s*mkp + c*mkq
		m[q][k] = m[k][q]
	}
	for k := range n {
		vkp, vkq := v[k][p], v[k][q]
		v[k][p] = c*vkp - s*vkq
		v[k][q] = s*vkp + c*vkq
	}
}

func sortEigenDescending(values []float64, vectors [][]float64) {
	n := len(values)
	for i := range n {
		best := i
		for j := i + 1; j < n; j++ {
			if values[j] > values[best] {
				best = j
			}
		}
		if best == i {
			continue
		}
		values[i], values[best] = values[best], values[i]
		for k := range n {
			vectors[k][i], vectors[k][best] = vectors[k][best], vectors[k][i]
		}
	}
}

func isSymmetric(a [][]float64) bool {
	n := len(a)
	if cols(a) != n {
		return false
	}
	for i := range n {
		for j := i + 1; j < n; j++ {
			if math.Abs(a[i][j]-a[j][i]) > 1e-9 {
				return false
			}
		}
	}
	return true
}

func zeroMatrix(rows, columns int) [][]float64 {
	m := make([][]float64, rows)
	for i := range m {
		m[i] = make([]float64, columns)
	}
	return m
}

func identityMatrix(n int) [][]float64 {
	m := zeroMatrix(n, n)
	for i := range n {
		m[i][i] = 1
	}
	return m
}

func copyMatrix(a [][]float64) [][]float64 {
	m := make([][]float64, len(a))
	for i, row := range a {
		m[i] = append([]float64(nil), row...)
	}
	return m
}
