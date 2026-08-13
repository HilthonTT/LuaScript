package docs

// The data-science stack: internal/native/datascience/*. These modules are
// registered alongside the rest in cmd/luascript/natives.go.

var datascienceTopics = []Topic{
	{
		Name: "ndarray", Kind: KindModule, RuntimeModule: "ndarray",
		Title:    "dense N-dimensional numeric arrays",
		Synopsis: `local nd = require("ndarray")`,
		Detail: `A NumPy-shaped array over a flat []float64, with broadcasting,
overloaded arithmetic operators, axis reductions and matrix multiply.

Broadcasting follows the NumPy rules: trailing dimensions must either
match or be 1. Operations that reduce to a single number — a vector dot
product, a full-array sum — return a plain Lua number rather than a
0-dimensional array.

Indices are 1-based, like the rest of the language. Axis arguments are
1-based too.`,
		Example: `local nd = require("ndarray")
local a = nd.array({{1, 2}, {3, 4}})
local b = nd.ones({2, 2})
print(a + b)                  -- broadcasting, __tostring
print(a:matmul(b):sum())
print(a:reshape({4}):mean())`,
		SeeAlso: []string{"ndarray.array", "linalg", "stats", "dataframe"},
		Entries: []Entry{
			{Name: "array", Kind: EntryFunction, Signature: "ndarray.array(t): ndarray",
				Summary: "Builds an array from a nested Lua table. The nesting depth becomes the number of dimensions."},
			{Name: "from_table", Kind: EntryFunction, Signature: "ndarray.from_table(t): ndarray",
				Summary: "Builds an array from a nested table — a synonym of ndarray.array."},
			{Name: "zeros", Kind: EntryFunction, Signature: "ndarray.zeros(shape): ndarray",
				Summary: "An array of the given shape filled with zeros. shape is a table of dimensions, e.g. {2, 3}."},
			{Name: "ones", Kind: EntryFunction, Signature: "ndarray.ones(shape): ndarray",
				Summary: "An array of the given shape filled with ones."},
			{Name: "full", Kind: EntryFunction, Signature: "ndarray.full(value, shape): ndarray",
				Summary: "An array of the given shape filled with value."},
			{Name: "eye", Kind: EntryFunction, Signature: "ndarray.eye(n): ndarray",
				Summary: "The n×n identity matrix."},
			{Name: "arange", Kind: EntryFunction, Signature: "ndarray.arange([start,] stop [, step]): ndarray",
				Summary: "A 1-D array of evenly spaced values over [start, stop), stepping by step (default 1)."},
			{Name: "linspace", Kind: EntryFunction, Signature: "ndarray.linspace(start, stop, n): ndarray",
				Summary: "A 1-D array of n values evenly spaced over the closed interval [start, stop]."},
			{Name: "matmul", Kind: EntryFunction, Signature: "ndarray.matmul(a, b): ndarray | number",
				Summary: "Matrix product of a and b. Two vectors give their dot product, as a plain number."},
			{Name: "concat", Kind: EntryFunction, Signature: "ndarray.concat(a, b [, axis]): ndarray",
				Summary: "Joins two arrays along an axis (default 1)."},
			{Name: "is_ndarray", Kind: EntryFunction, Signature: "ndarray.is_ndarray(v): boolean",
				Summary: "Reports whether v is an ndarray rather than an ordinary table."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "ndarray.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "dataframe", Kind: KindModule, RuntimeModule: "dataframe",
		Title:    "columnar data frames (pandas-lite)",
		Synopsis: `local dataframe = require("dataframe")`,
		Detail: `A column-oriented table: each column is a named array, and every
column has the same length. Operations return new frames rather than
mutating in place, so they chain.

Printing a frame renders an aligned table, courtesy of __tostring.`,
		Example: `local dataframe = require("dataframe")
local df = dataframe.new({ name = {"ada", "bob"}, age = {36, 41} })
print(df:filter(function(row) return row.age > 38 end))
print(df:sort("age"):head(1))`,
		SeeAlso: []string{"dataframe.frame", "csv", "stats", "ndarray"},
		Entries: []Entry{
			{Name: "new", Kind: EntryFunction, Signature: "dataframe.new(columns): frame",
				Summary: "Builds a frame from a table of named columns, each an array of equal length."},
			{Name: "from_rows", Kind: EntryFunction, Signature: "dataframe.from_rows(rows): frame",
				Summary: "Builds a frame from an array of row tables, taking the column names from the keys."},
			{Name: "from_csv", Kind: EntryFunction, Signature: "dataframe.from_csv(path): frame",
				Summary: "Reads a CSV file into a frame, using the header line as the column names."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "dataframe.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "csv", Kind: KindModule, RuntimeModule: "csv",
		Title:    "CSV reading and writing",
		Synopsis: `local csv = require("csv")`,
		Detail: `Rows are arrays of strings — no type inference happens here. For typed
columns and column-wise operations, load the file with
dataframe.from_csv instead.`,
		Example: `local csv = require("csv")
local rows = csv.read("data.csv")
print(#rows, rows[1][1])
csv.write("out.csv", rows)`,
		SeeAlso: []string{"dataframe", "json", "io"},
		Entries: []Entry{
			{Name: "read", Kind: EntryFunction, Signature: "csv.read(path): table",
				Summary: "Reads a CSV file into an array of row arrays."},
			{Name: "write", Kind: EntryFunction, Signature: "csv.write(path, rows)",
				Summary: "Writes an array of row arrays to a CSV file."},
			{Name: "parse", Kind: EntryFunction, Signature: "csv.parse(text): table",
				Summary: "Parses CSV text already in memory into an array of row arrays."},
			{Name: "stringify", Kind: EntryFunction, Signature: "csv.stringify(rows): string",
				Summary: "Renders an array of row arrays as CSV text, quoting where needed."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "csv.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "stats", Kind: KindModule, RuntimeModule: "stats",
		Title:    "descriptive statistics",
		Synopsis: `local stats = require("stats")`,
		Detail: `Every function takes an array of numbers. Note the sample/population
split: variance and stddev divide by n-1 (sample), pvariance and pstddev
divide by n (population).`,
		Example: `local stats = require("stats")
local xs = { 2, 4, 4, 4, 5, 5, 7, 9 }
print(stats.mean(xs), stats.median(xs), stats.stddev(xs))
local d = stats.describe(xs)
print(d.count, d.q1, d.q3)`,
		SeeAlso: []string{"math", "dataframe", "linalg"},
		Entries: []Entry{
			{Name: "mean", Kind: EntryFunction, Signature: "stats.mean(xs): number", Summary: "The arithmetic mean."},
			{Name: "median", Kind: EntryFunction, Signature: "stats.median(xs): number", Summary: "The middle value, averaging the two middle values when the count is even."},
			{Name: "mode", Kind: EntryFunction, Signature: "stats.mode(xs): number", Summary: "The most frequent value."},
			{Name: "min", Kind: EntryFunction, Signature: "stats.min(xs): number", Summary: "The smallest value."},
			{Name: "max", Kind: EntryFunction, Signature: "stats.max(xs): number", Summary: "The largest value."},
			{Name: "range", Kind: EntryFunction, Signature: "stats.range(xs): number", Summary: "The difference between the largest and smallest values."},
			{Name: "sum", Kind: EntryFunction, Signature: "stats.sum(xs): number", Summary: "The sum of the values."},
			{Name: "product", Kind: EntryFunction, Signature: "stats.product(xs): number", Summary: "The product of the values."},
			{Name: "cumsum", Kind: EntryFunction, Signature: "stats.cumsum(xs): table", Summary: "The running totals of xs, as an array of the same length."},
			{Name: "variance", Kind: EntryFunction, Signature: "stats.variance(xs): number", Summary: "The sample variance, dividing by n-1."},
			{Name: "stddev", Kind: EntryFunction, Signature: "stats.stddev(xs): number", Summary: "The sample standard deviation, dividing by n-1."},
			{Name: "pvariance", Kind: EntryFunction, Signature: "stats.pvariance(xs): number", Summary: "The population variance, dividing by n."},
			{Name: "pstddev", Kind: EntryFunction, Signature: "stats.pstddev(xs): number", Summary: "The population standard deviation, dividing by n."},
			{Name: "sem", Kind: EntryFunction, Signature: "stats.sem(xs): number", Summary: "The standard error of the mean."},
			{Name: "quantile", Kind: EntryFunction, Signature: "stats.quantile(xs, q): number", Summary: "The quantile at q, with q in [0, 1]."},
			{Name: "percentile", Kind: EntryFunction, Signature: "stats.percentile(xs, p): number", Summary: "The percentile at p, with p in [0, 100]."},
			{Name: "iqr", Kind: EntryFunction, Signature: "stats.iqr(xs): number", Summary: "The interquartile range, q3 minus q1."},
			{Name: "skewness", Kind: EntryFunction, Signature: "stats.skewness(xs): number", Summary: "The asymmetry of the distribution."},
			{Name: "kurtosis", Kind: EntryFunction, Signature: "stats.kurtosis(xs): number", Summary: "The tailedness of the distribution."},
			{Name: "geomean", Kind: EntryFunction, Signature: "stats.geomean(xs): number", Summary: "The geometric mean."},
			{Name: "harmonic_mean", Kind: EntryFunction, Signature: "stats.harmonic_mean(xs): number", Summary: "The harmonic mean."},
			{Name: "correlation", Kind: EntryFunction, Signature: "stats.correlation(xs, ys): number",
				Summary: "The Pearson correlation coefficient of two equally long series."},
			{Name: "covariance", Kind: EntryFunction, Signature: "stats.covariance(xs, ys): number",
				Summary: "The covariance of two equally long series."},
			{Name: "normalize", Kind: EntryFunction, Signature: "stats.normalize(xs): table",
				Summary: "Rescales the values to [0, 1] — min-max normalisation."},
			{Name: "standardize", Kind: EntryFunction, Signature: "stats.standardize(xs): table",
				Summary: "Rescales the values to zero mean and unit variance."},
			{Name: "zscore", Kind: EntryFunction, Signature: "stats.zscore(xs): table",
				Summary: "The z-score of each value: how many standard deviations it sits from the mean."},
			{Name: "spearman", Kind: EntryFunction, Signature: "stats.spearman(xs, ys): number",
				Summary: "The Spearman rank correlation of two equally long series.",
				Detail: `Pearson computed on the ranks, so it detects any monotonic
relationship rather than only a linear one, and outliers cannot drag it
around. Tied values share the mean of their ranks.`},
			{Name: "weighted_mean", Kind: EntryFunction, Signature: "stats.weighted_mean(xs, weights): number",
				Summary: "The mean of xs with each value weighted by the matching entry in weights."},
			{Name: "histogram", Kind: EntryFunction, Signature: "stats.histogram(xs [, bins]): table",
				Summary: "Bins the values, returning { counts, edges }. bins defaults to 10.",
				Detail:  "edges has one more entry than counts. The maximum value falls in the last bin rather than in one of its own."},
			{Name: "normal_pdf", Kind: EntryFunction, Signature: "stats.normal_pdf(x [, mu [, sigma]]): number",
				Summary: "The normal probability density at x. mu defaults to 0 and sigma to 1."},
			{Name: "normal_cdf", Kind: EntryFunction, Signature: "stats.normal_cdf(x [, mu [, sigma]]): number",
				Summary: "The normal cumulative probability up to x — the area to the left. mu defaults to 0 and sigma to 1."},
			{Name: "t_test_1sample", Kind: EntryFunction, Signature: "stats.t_test_1sample(xs [, mu]): table",
				Summary: "Tests whether the mean of xs differs from mu, returning { t, df, p }. mu defaults to 0.",
				Detail:  "p is two-tailed. Needs at least two values and a non-zero variance."},
			{Name: "t_test_2sample", Kind: EntryFunction, Signature: "stats.t_test_2sample(xs, ys): table",
				Summary: "Tests whether two samples have different means, returning { t, df, p }.",
				Detail: `Welch's t-test: it does not assume the two groups share a variance,
which is the assumption most often violated in practice. df is
therefore fractional. p is two-tailed.`},
			{Name: "describe", Kind: EntryFunction, Signature: "stats.describe(xs): table",
				Summary: "A summary table with count, mean, std, min, q1, median, q3 and max."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "stats.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "linalg", Kind: KindModule, RuntimeModule: "linalg",
		Title:    "linear algebra over plain tables",
		Synopsis: `local linalg = require("linalg")`,
		Detail: `Matrices are arrays of row arrays and vectors are flat arrays — plain
Lua tables throughout, so results interoperate with the rest of the
language without wrapping.

For large or repeated numeric work, ndarray is faster: it stores one flat
[]float64 instead of a table per row.`,
		Example: `local linalg = require("linalg")
local a = { {1, 2}, {3, 4} }
print(linalg.det(a))
local x = linalg.solve(a, {5, 11})`,
		SeeAlso: []string{"ndarray", "stats", "math"},
		Entries: []Entry{
			{Name: "add", Kind: EntryFunction, Signature: "linalg.add(a, b): table", Summary: "Element-wise sum of two matrices."},
			{Name: "sub", Kind: EntryFunction, Signature: "linalg.sub(a, b): table", Summary: "Element-wise difference of two matrices."},
			{Name: "scale", Kind: EntryFunction, Signature: "linalg.scale(a, k): table", Summary: "Multiplies every element of a by the scalar k."},
			{Name: "matmul", Kind: EntryFunction, Signature: "linalg.matmul(a, b): table", Summary: "Matrix product of a and b."},
			{Name: "matvec", Kind: EntryFunction, Signature: "linalg.matvec(a, v): table", Summary: "Product of a matrix and a vector."},
			{Name: "dot", Kind: EntryFunction, Signature: "linalg.dot(u, v): number", Summary: "Dot product of two vectors."},
			{Name: "norm", Kind: EntryFunction, Signature: "linalg.norm(v): number", Summary: "The Euclidean length of a vector."},
			{Name: "distance", Kind: EntryFunction, Signature: "linalg.distance(u, v): number", Summary: "The Euclidean distance between two vectors."},
			{Name: "transpose", Kind: EntryFunction, Signature: "linalg.transpose(a): table", Summary: "The transpose of a matrix."},
			{Name: "det", Kind: EntryFunction, Signature: "linalg.det(a): number", Summary: "The determinant of a square matrix."},
			{Name: "inverse", Kind: EntryFunction, Signature: "linalg.inverse(a): table",
				Summary: "The inverse of a square matrix. Raises when the matrix is singular."},
			{Name: "solve", Kind: EntryFunction, Signature: "linalg.solve(a, b): table",
				Summary: "Solves the linear system Ax = b for x. A must be square and non-singular."},
			{Name: "lstsq", Kind: EntryFunction, Signature: "linalg.lstsq(a, b): table",
				Summary: "The least-squares solution of Ax = b, for the overdetermined systems solve cannot take.",
				Detail: `The usual shape of a regression: more observations than parameters.
Computed through QR rather than the normal equations, which would square
the condition number and lose roughly half the available precision.`},
			{Name: "qr", Kind: EntryFunction, Signature: "linalg.qr(a): table, table",
				Summary: "The QR decomposition: an orthonormal Q and upper-triangular R with A = Q*R.",
				Detail:  "Needs at least as many rows as columns, and raises when the columns are linearly dependent."},
			{Name: "cholesky", Kind: EntryFunction, Signature: "linalg.cholesky(a): table",
				Summary: "The lower-triangular L with L*Lt = A. Requires A symmetric positive definite.",
				Detail: `About twice as fast as LU, and the standard route for covariance
matrices. The failure case is useful in its own right: it raises exactly
when A is not positive definite.`},
			{Name: "eigh", Kind: EntryFunction, Signature: "linalg.eigh(a): table, table",
				Summary: "Eigenvalues and eigenvectors of a symmetric matrix, largest first.",
				Detail: `Returns the values as an array and the vectors as a matrix whose
columns match them — the layout PCA and covariance analysis consume.
Raises when A is not symmetric.`},
			{Name: "rank", Kind: EntryFunction, Signature: "linalg.rank(a): number",
				Summary: "The number of linearly independent rows.",
				Detail:  "Use it to find out whether a system is solvable before solve raises on it."},
			{Name: "trace", Kind: EntryFunction, Signature: "linalg.trace(a): number", Summary: "The sum of the diagonal entries."},
			{Name: "identity", Kind: EntryFunction, Signature: "linalg.identity(n): table", Summary: "The n×n identity matrix."},
			{Name: "zeros", Kind: EntryFunction, Signature: "linalg.zeros(rows, cols): table", Summary: "A matrix of zeros."},
			{Name: "ones", Kind: EntryFunction, Signature: "linalg.ones(rows, cols): table", Summary: "A matrix of ones."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "linalg.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "clustering", Kind: KindModule, RuntimeModule: "clustering",
		Title:    "unsupervised clustering",
		Synopsis: `local clustering = require("clustering")`,
		Detail: `Data is an array of points, each point an array of coordinates.
Results come back as a table with centroids, assignments (1-based
cluster indices, parallel to the input) and iterations.

DBSCAN labels noise points rather than forcing them into a cluster.`,
		Example: `local clustering = require("clustering")
local pts = { {1, 1}, {1.2, 0.9}, {8, 8}, {8.1, 7.9} }
local r = clustering.kmeans(pts, 2)
print(r.iterations, r.assignments[1], r.assignments[3])`,
		SeeAlso: []string{"classification", "stats", "ml"},
		Entries: []Entry{
			{Name: "kmeans", Kind: EntryFunction, Signature: "clustering.kmeans(points, k [, opts]): table",
				Summary: "Partitions the points into k clusters by Lloyd's algorithm."},
			{Name: "dbscan", Kind: EntryFunction, Signature: "clustering.dbscan(points, eps, min_pts): table",
				Summary: "Density-based clustering: groups points within eps that have at least min_pts neighbours, and labels the rest as noise."},
			{Name: "hierarchical", Kind: EntryFunction, Signature: "clustering.hierarchical(points, k): table",
				Summary: "Agglomerative clustering, merging until k clusters remain."},
			{Name: "meanshift", Kind: EntryFunction, Signature: "clustering.meanshift(points, bandwidth): table",
				Summary: "Mode-seeking clustering with the given kernel bandwidth. The cluster count comes out of the data."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "clustering.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "classification", Kind: KindModule, RuntimeModule: "classification",
		Title:    "supervised classifiers",
		Synopsis: `local classification = require("classification")`,
		Detail: `Each constructor returns a model object. The numeric models (knn,
perceptron, logistic, svm) share a fit/predict interface: :fit(features,
labels) with features an array of coordinate arrays, then :predict(x).
logistic and the others also offer :predict_proba.

naivebayes is text-oriented and has its own interface: :learn(doc,
class), :classify(doc) and :classifyProb(doc).`,
		Example: `local classification = require("classification")
local knn = classification.knn(3)
knn:fit({ {1, 1}, {2, 2}, {8, 8} }, { "a", "a", "b" })
print(knn:predict({1.5, 1.5}))`,
		SeeAlso: []string{"clustering", "ml", "stats"},
		Entries: []Entry{
			{Name: "knn", Kind: EntryFunction, Signature: "classification.knn([k]): model",
				Summary: "A k-nearest-neighbours classifier (k defaults to 3)."},
			{Name: "logistic", Kind: EntryFunction, Signature: "classification.logistic([opts]): model",
				Summary: "Logistic regression, with :fit, :predict and :predict_proba."},
			{Name: "perceptron", Kind: EntryFunction, Signature: "classification.perceptron([opts]): model",
				Summary: "A single-layer perceptron, with :fit and :predict."},
			{Name: "svm", Kind: EntryFunction, Signature: "classification.svm([opts]): model",
				Summary: "A support vector machine, with :fit, :predict, :decision_function and :support_vectors."},
			{Name: "naivebayes", Kind: EntryFunction, Signature: "classification.naivebayes(...): model",
				Summary: "A naive Bayes text classifier over the named classes, with :learn, :classify, :classifyProb, :classes and :learned."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "classification.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "ml", Kind: KindModule, RuntimeModule: "ml",
		Title:    "feed-forward neural networks",
		Synopsis: `local ml = require("ml")`,
		Detail: `A small dense network trained by backpropagation. ml.new takes a
config table describing the layer sizes and hyperparameters; the
resulting net has :train, :predict, :save and :save_file, and can be
brought back with ml.load or ml.load_file.

See examples/41_ml_module.lsc for a full run.`,
		SeeAlso: []string{"ml.net", "classification", "ndarray"},
		Entries: []Entry{
			{Name: "new", Kind: EntryFunction, Signature: "ml.new(config): net",
				Summary: "Creates a network from a config table (layer sizes, learning rate, activation)."},
			{Name: "load", Kind: EntryFunction, Signature: "ml.load(blob): net",
				Summary: "Restores a network from a string produced by net:save."},
			{Name: "load_file", Kind: EntryFunction, Signature: "ml.load_file(path): net",
				Summary: "Restores a network from a file written by net:save_file."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "ml.VERSION: string", Summary: "The module's version string."},
		},
	},
	{
		Name: "plot", Kind: KindModule, RuntimeModule: "plot",
		Title:    "dependency-free SVG charting",
		Synopsis: `local plot = require("plot")`,
		Detail: `Charts render to SVG text with no external dependencies — no fonts to
install, no headless browser. Axes are auto-ranged onto 1-2-5 "nice"
ticks and a legend appears when a series is named.

The module-level line/scatter/bar/histogram are shorthands that create a
figure with one series already added. For several series, start from
plot.figure() and chain: every figure method returns the figure.`,
		Example: `local plot = require("plot")
plot.figure()
  :line({1, 2, 3}, {2, 4, 8}, "growth")
  :title("demo"):xlabel("x"):ylabel("y")
  :save("out.svg")`,
		SeeAlso: []string{"plot.figure", "dataframe", "stats", "ui"},
		Entries: []Entry{
			{Name: "figure", Kind: EntryFunction, Signature: "plot.figure(): figure",
				Summary: "Creates an empty figure. See the plot.figure page for its methods."},
			{Name: "line", Kind: EntryFunction, Signature: "plot.line(xs, ys [, label]): figure",
				Summary: "A figure holding one line series."},
			{Name: "scatter", Kind: EntryFunction, Signature: "plot.scatter(xs, ys [, label]): figure",
				Summary: "A figure holding one scatter series."},
			{Name: "bar", Kind: EntryFunction, Signature: "plot.bar(labels, values [, label]): figure",
				Summary: "A figure holding one bar series."},
			{Name: "histogram", Kind: EntryFunction, Signature: "plot.histogram(values [, bins [, label]]): figure",
				Summary: "A figure holding a histogram of values."},
			{Name: "VERSION", Kind: EntryConstant, Signature: "plot.VERSION: string", Summary: "The module's version string."},
		},
	},
}
