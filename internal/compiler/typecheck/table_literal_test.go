package typecheck

import "testing"

func TestTableLiteralFieldTypeChecked(t *testing.T) {
	expectErrContains(t, `
		type User = { name: string, age: number }
		local u: User = { name = "Ada", age = "36" }`,
		`Type "string" could not be converted into "number" in field "age" at line 3`)
}

func TestTableLiteralMissingRequiredField(t *testing.T) {
	expectErrContains(t, `
		type Point = { x: number, y: number }
		local p: Point = { x = 1 }`,
		`missing required field "y" of type "number"`)
}

func TestTableLiteralOptionalFieldMayBeOmitted(t *testing.T) {
	expectOK(t, `
		type User = { name: string, nick: string? }
		local u: User = { name = "Ada" }`)
}

func TestTableLiteralExtraFieldRejected(t *testing.T) {
	expectErrContains(t, `
		type User = { name: string, nick: string? }
		local u: User = { name = "Ada", nikc = "A" }`,
		`table literal has field "nikc"`)
}

func TestTableLiteralGenericAlias(t *testing.T) {
	expectErrContains(t, `
		type Box<T> = { value: T }
		local b: Box<number> = { value = "oops" }`,
		`could not be converted into "number" in field "value"`)
	expectOK(t, `
		type Box<T> = { value: T }
		local b: Box<Box<number>> = { value = { value = 1 } }`)
}

func TestTableLiteralArrayElements(t *testing.T) {
	expectErrContains(t, `
		local xs: { number } = { 1, "two" }`,
		`in field "[2]"`)
	expectOK(t, `
		local xs: { number } = { 1, 2, 3 }`)
}

func TestTableLiteralMapEntries(t *testing.T) {
	expectErrContains(t, `
		local m: { [string]: number } = { a = 1, b = "x" }`,
		`in field "b"`)
	expectOK(t, `
		local m: { [string]: number } = { a = 1, ["b c"] = 2 }`)
}

func TestTableLiteralNestedRecords(t *testing.T) {
	expectErrContains(t, `
		type Address = { city: string }
		type User = { name: string, address: Address }
		local u: User = {
			name = "Ada",
			address = { city = 42 },
		}`,
		`in field "address.city" at line 6`)
}

func TestTableLiteralAnyAndUntypedStayUnchecked(t *testing.T) {
	expectOK(t, `
		local a: any = { 1, "two", x = true }
		local b = { 1, "two", x = true }
		local function f(): any return 1 end
		type User = { name: string }
		local u: User = { name = f() }`)
}

func TestTableLiteralSpreadIsPermissive(t *testing.T) {
	expectOK(t, `
		type Point = { x: number, y: number }
		local base = { x = 1, y = 2 }
		local p: Point = { ...base, x = 3 }`)
}

func TestTableLiteralEmptyIsAllowed(t *testing.T) {
	expectOK(t, `
		type Point = { x: number, y: number }
		local p: Point = {}
		p.x = 1
		p.y = 2`)
}

func TestTableLiteralCallArgument(t *testing.T) {
	expectErrContains(t, `
		type Point = { x: number, y: number }
		local function len(p: Point): number return p.x + p.y end
		print(len({ x = 1, y = "2" }))`,
		`in field "y"`)
}

func TestTableLiteralReturnValue(t *testing.T) {
	expectErrContains(t, `
		type Point = { x: number, y: number }
		local function origin(): Point
			return { x = 0 }
		end`,
		`missing required field "y"`)
}

func TestTableLiteralFieldAssignment(t *testing.T) {
	expectErrContains(t, `
		type Address = { city: string }
		type User = { name: string, address: Address }
		local u: User = { name = "Ada", address = { city = "London" } }
		u.address = { city = false }`,
		`in field "city"`)
}

func TestTableLiteralAssignmentToAnnotatedLocal(t *testing.T) {
	expectErrContains(t, `
		type Point = { x: number, y: number }
		local p: Point = { x = 1, y = 2 }
		p = { x = 1 }`,
		`missing required field "y"`)
}

func TestTableLiteralOptionalTarget(t *testing.T) {
	expectErrContains(t, `
		type Point = { x: number, y: number }
		local p: Point? = { x = 1, y = "2" }`,
		`in field "y"`)
}

func TestTableLiteralDiscriminatedUnion(t *testing.T) {
	src := `
		type Shape = { kind: "circle", r: number } | { kind: "rect", w: number, h: number }
		local a: Shape = { kind = "circle", r = 1 }
		local b: Shape = { kind = "rect", w = 1, h = 2 }`
	expectOK(t, src)
	expectErrContains(t, `
		type Shape = { kind: "circle", r: number } | { kind: "rect", w: number, h: number }
		local c: Shape = { kind = "square", side = 1 }`,
		`could not be converted into`)
}

func TestTableLiteralAgainstNonTable(t *testing.T) {
	expectErrContains(t, `
		local n: number = { 1 }`,
		`could not be converted into "number"`)
}

func TestTableLiteralStructAliasIgnoresImplMembers(t *testing.T) {
	expectOK(t, `
		struct Point { x: number, y: number }
		impl Point
			function len(self): number return self.x + self.y end
		end
		local p: Point = { x = 1, y = 2 }`)
}

func TestTableLiteralFunctionFieldsCheckedLoosely(t *testing.T) {
	expectOK(t, `
		type Shape = { name: string, area: (any) -> number }
		local s: Shape = {
			name = "unit",
			area = function(self) return 1 end,
		}`)
}

func TestTableLiteralGenericCallUnchanged(t *testing.T) {
	expectOK(t, `
		local function first<T>(xs: { T }): T return xs[1] end
		local n = first({ 1, 2, 3 })`)
}

func TestTableLiteralDefaultParam(t *testing.T) {
	expectErrContains(t, `
		type Opts = { verbose: boolean }
		local function run(o: Opts = { verbose = 1 }) end`,
		`in field "verbose"`)
}

func TestAndOrTernaryIdiomIsNumber(t *testing.T) {
	expectOK(t, `
		local i = 3
		local n: number = 2 * i + (i % 2 == 0 and 0.1 or -0.1)`)
}

func TestShadowedRequireReturnsAny(t *testing.T) {
	expectOK(t, `
		local require = function(n) return {} end
		local x = require("json").encodee`)
	expectOK(t, `
		require = function(n) return {} end
		local x = require("json").encodee`)
	expectErrContains(t, `
		local x = require("json").encodee`,
		`has no field "encodee"`)
}

func TestStandardGlobalsAreTyped(t *testing.T) {
	expectErrContains(t, `local x = os.getenvv("HOME")`, `has no field "getenvv"`)
	expectErrContains(t, `local x = utf8.lenn("abc")`, `has no field "lenn"`)
	expectErrContains(t, `local n: number = os.getenv("HOME")`, `could not be converted`)
	expectOK(t, `
		local home = os.getenv("HOME")
		local n = utf8.len("héllo")
		io.write("x")`)
}

func TestStandardGlobalsCanBeShadowed(t *testing.T) {
	expectOK(t, `
		local os = require("os")
		local t = os.time()`)
	expectOK(t, `
		local os = { custom = 1 }
		local n = os.custom`)
}

func TestNativeSignaturesAcceptRuntimeOptions(t *testing.T) {
	expectOK(t, `
		local clustering = require("clustering")
		local classification = require("classification")
		local compression = require("compression")
		local data = { { 1, 2 }, { 2, 3 } }
		local a = clustering.kmeans(data, 2, { seed = 1 })
		local b = clustering.dbscan(data, 0.5, 2)
		local c = clustering.hierarchical(data, 2, { linkage = "single" })
		local d = clustering.meanshift(data, 1.5, { max_iter = 10 })
		local knn = classification.knn(3)
		local lr = classification.logistic({ epochs = 10 })
		local nb = classification.naivebayes("spam", "ham", { tfidf = true })
		local z = compression.gzip_encode("hello", 9)
		local enc = compression.encode("hello")
		local bits: string = enc.bits
		local plain: string = compression.decode(enc)`)
}
