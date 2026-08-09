package docs

// Object types — values returned by a constructor, documented for their
// methods. Names are dotted with the module that produces them so
// `luascript doc std.stack` reads naturally and cannot collide with a
// module-level entry.
//
// Methods are called with a colon (obj:method(...)); the receiver is not
// shown in the signatures below.

var objectTopics = []Topic{
	{
		Name: "io.file", Kind: KindObject, RuntimeModule: "io",
		Title:    "stream-oriented file handle (io.open)",
		Synopsis: `local f = assert(require("io").open("data.txt", "r"))`,
		Detail: `The handle io.open, io.tmpfile and the standard streams return.
Calling a method on a closed handle raises.

os.open returns a DIFFERENT type — see os.file — which is
descriptor-oriented rather than stream-oriented.`,
		Example: `local io = require("io")
local f = assert(io.open("data.txt", "r"))
for line in f:lines() do print(line) end
f:close()`,
		SeeAlso: []string{"io", "os.file"},
		Entries: []Entry{
			{Name: "read", Kind: EntryMethod, Signature: `f:read([fmt]): string?`,
				Summary: `Reads by format: "l" (default) a line without its newline, "L" with it, "n" a number, "a" the rest of the file. A number reads that many bytes. Returns nil at end of file.`},
			{Name: "write", Kind: EntryMethod, Signature: "f:write(...): file",
				Summary: "Writes each argument with no separator, and returns the handle so calls chain."},
			{Name: "lines", Kind: EntryMethod, Signature: "f:lines([fmt]): iterator",
				Summary: "Returns an iterator over the remaining lines. Unlike io.lines, it does not close the handle when exhausted."},
			{Name: "seek", Kind: EntryMethod, Signature: `f:seek([whence [, offset]]): number`,
				Summary: `Moves and reports the file position. whence is "set", "cur" (default) or "end"; the return is the new position from the start.`},
			{Name: "flush", Kind: EntryMethod, Signature: "f:flush(): file",
				Summary: "Flushes buffered writes to the operating system."},
			{Name: "close", Kind: EntryMethod, Signature: "f:close(): boolean",
				Summary: "Closes the handle. Further method calls on it raise."},
		},
	},
	{
		Name: "os.file", Kind: KindObject, RuntimeModule: "os",
		Title:    "descriptor-oriented file handle (os.open)",
		Synopsis: `local f = require("os").open(path, flags, perm)`,
		Detail: `What os.open and os.create return. It sits closer to the system call
layer than io.file: seeks take a numeric whence constant, and it can
stat, chmod and truncate the underlying file.`,
		Example: `local os = require("os")
local f = os.open("/tmp/x", os.o_rdwr | os.o_create, 0644)
f:write("hello")
f:seek(0, os.seek_set)
print(f:read(5))
print(f:stat().size)
f:close()`,
		SeeAlso: []string{"os", "io.file"},
		Entries: []Entry{
			{Name: "read", Kind: EntryMethod, Signature: "f:read(n): string",
				Summary: "Reads up to n bytes from the current position."},
			{Name: "write", Kind: EntryMethod, Signature: "f:write(s): number",
				Summary: "Writes a string at the current position and returns the byte count."},
			{Name: "seek", Kind: EntryMethod, Signature: "f:seek(offset, whence): number",
				Summary: "Moves the file position; whence is os.seek_set, os.seek_cur or os.seek_end. Returns the new position."},
			{Name: "stat", Kind: EntryMethod, Signature: "f:stat(): table",
				Summary: "File metadata: name, size, mode, mod_time and is_dir."},
			{Name: "chmod", Kind: EntryMethod, Signature: "f:chmod(mode): boolean",
				Summary: "Changes the file's permission bits."},
			{Name: "truncate", Kind: EntryMethod, Signature: "f:truncate(size): boolean",
				Summary: "Resizes the file, padding with zeros when it grows."},
			{Name: "sync", Kind: EntryMethod, Signature: "f:sync(): boolean",
				Summary: "Forces buffered writes all the way to stable storage."},
			{Name: "name", Kind: EntryMethod, Signature: "f:name(): string",
				Summary: "The path the file was opened with."},
			{Name: "close", Kind: EntryMethod, Signature: "f:close(): boolean",
				Summary: "Closes the descriptor."},
		},
	},
	{
		Name: "regexp.regex", Kind: KindObject, RuntimeModule: "regexp",
		Title:    "compiled RE2 pattern",
		Synopsis: `local re = require("regexp").compile("\\d+")`,
		Detail: `Compile once, use many times — compilation is the expensive part.
The method is :capture rather than :match because match is a keyword.`,
		SeeAlso: []string{"regexp", "string"},
		Entries: []Entry{
			{Name: "test", Kind: EntryMethod, Signature: "re:test(s): boolean",
				Summary: "Reports whether the pattern matches anywhere in s."},
			{Name: "capture", Kind: EntryMethod, Signature: "re:capture(s): ...string",
				Summary: "Returns the capture groups of the first match, or nil when there is none."},
			{Name: "find_all", Kind: EntryMethod, Signature: "re:find_all(s [, n]): table",
				Summary: "Every match in s, as an array. n caps the number returned."},
			{Name: "replace", Kind: EntryMethod, Signature: "re:replace(s, repl): string",
				Summary: "Replaces every match in s. Use $1, $2 in repl to reference capture groups."},
			{Name: "split", Kind: EntryMethod, Signature: "re:split(s [, n]): table",
				Summary: "Splits s around each match, returning the pieces as an array."},
		},
	},
	{
		Name: "http.client", Kind: KindObject, RuntimeModule: "http",
		Title:    "reusable HTTP client",
		Synopsis: `local c = require("http").new_client({ base_url = "https://api.example.com" })`,
		Detail: `Shares a connection pool, a base URL and default headers across
calls. Request paths are resolved against base_url, and per-call headers
override the client's. Responses have the same shape as the module-level
shorthands — see the http page.`,
		SeeAlso: []string{"http", "json"},
		Entries: []Entry{
			{Name: "get", Kind: EntryMethod, Signature: "c:get(url [, opts]): table", Summary: "GET, resolved against base_url."},
			{Name: "head", Kind: EntryMethod, Signature: "c:head(url [, opts]): table", Summary: "HEAD, resolved against base_url."},
			{Name: "options", Kind: EntryMethod, Signature: "c:options(url [, opts]): table", Summary: "OPTIONS, resolved against base_url."},
			{Name: "delete", Kind: EntryMethod, Signature: "c:delete(url [, opts]): table", Summary: "DELETE, resolved against base_url."},
			{Name: "post", Kind: EntryMethod, Signature: "c:post(url [, body [, opts]]): table", Summary: "POST with a body string."},
			{Name: "put", Kind: EntryMethod, Signature: "c:put(url [, body [, opts]]): table", Summary: "PUT with a body string."},
			{Name: "patch", Kind: EntryMethod, Signature: "c:patch(url [, body [, opts]]): table", Summary: "PATCH with a body string."},
			{Name: "request", Kind: EntryMethod, Signature: "c:request(opts): table",
				Summary: "The full-surface form, taking the same options table as http.request."},
		},
	},
	{
		Name: "httpserver.server", Kind: KindObject, RuntimeModule: "httpserver",
		Title:    "HTTP server instance",
		Synopsis: `local server = require("httpserver").new()`,
		Detail: `Handlers receive a request table with method, path, query, body, host,
remote_addr and headers, and return a response table with status, body
and headers. Register routes before calling :listen — it blocks.`,
		Example: `local server = require("httpserver").new()
server:get("/health", function(req) return { status = 200, body = "ok" } end)
server:set_not_found(function(req) return { status = 404, body = "nope" } end)
server:listen(":8080")`,
		SeeAlso: []string{"httpserver", "http", "json"},
		Entries: []Entry{
			{Name: "route", Kind: EntryMethod, Signature: "server:route(method, path, handler)",
				Summary: "Registers a handler for one method and path. The get/post/... methods are shorthands for this."},
			{Name: "get", Kind: EntryMethod, Signature: "server:get(path, handler)", Summary: "Registers a GET route."},
			{Name: "post", Kind: EntryMethod, Signature: "server:post(path, handler)", Summary: "Registers a POST route."},
			{Name: "put", Kind: EntryMethod, Signature: "server:put(path, handler)", Summary: "Registers a PUT route."},
			{Name: "patch", Kind: EntryMethod, Signature: "server:patch(path, handler)", Summary: "Registers a PATCH route."},
			{Name: "delete", Kind: EntryMethod, Signature: "server:delete(path, handler)", Summary: "Registers a DELETE route."},
			{Name: "head", Kind: EntryMethod, Signature: "server:head(path, handler)", Summary: "Registers a HEAD route."},
			{Name: "options", Kind: EntryMethod, Signature: "server:options(path, handler)", Summary: "Registers an OPTIONS route."},
			{Name: "set_not_found", Kind: EntryMethod, Signature: "server:set_not_found(handler)",
				Summary: "Sets the handler used when no route matches."},
			{Name: "listen", Kind: EntryMethod, Signature: `server:listen(addr)`,
				Summary: `Binds addr (e.g. ":8080") and serves until :stop() is called. Blocks the VM goroutine, dispatching handlers one at a time.`},
			{Name: "stop", Kind: EntryMethod, Signature: "server:stop()",
				Summary: "Shuts the server down; the blocked :listen returns cleanly. Safe to call from a handler."},
		},
	},
	{
		Name: "db.conn", Kind: KindObject, RuntimeModule: "db",
		Title:    "database connection pool",
		Synopsis: `local conn = require("db").open(driver, datasource)`,
		Detail: `Wraps a *sql.DB, so it is a pool rather than a single connection.
Pass query parameters as extra arguments — the driver escapes them.

conn.driver is the resolved database/sql driver name, so a script that
has to care about dialect differences can branch on it, and
conn:placeholder(n) gives that driver's bind-parameter syntax.`,
		SeeAlso: []string{"db", "json"},
		Entries: []Entry{
			{Name: "query", Kind: EntryMethod, Signature: "conn:query(sql, ...): table",
				Summary: "Runs a query and returns the rows as an array of tables keyed by column name."},
			{Name: "exec", Kind: EntryMethod, Signature: "conn:exec(sql, ...): number, number",
				Summary: "Runs a statement that returns no rows; returns rows affected and the last insert id.",
				Detail:  "Both are best-effort: Postgres reports 0 for the insert id (use RETURNING and :query instead), and some drivers report 0 rows affected for DDL."},
			{Name: "placeholder", Kind: EntryMethod, Signature: "conn:placeholder([n]): string",
				Summary: "This connection's bind-parameter syntax for the nth parameter. n defaults to 1."},
			{Name: "driver", Kind: EntryField, Signature: "conn.driver: string",
				Summary: "The resolved database/sql driver name backing this connection."},
			{Name: "ping", Kind: EntryMethod, Signature: "conn:ping(): boolean, string?",
				Summary: "Verifies the connection is alive, opening one if the pool is empty."},
			{Name: "close", Kind: EntryMethod, Signature: "conn:close(): boolean",
				Summary: "Closes the pool and every connection in it."},
		},
	},
	{
		Name: "queue.jobqueue", Kind: KindObject, RuntimeModule: "queue",
		Title:    "priority job queue instance",
		Synopsis: `local q = require("queue").new()`,
		Detail: `Jobs run on the VM goroutine only, one at a time — see the queue page
for why. :push is safe to call from a job; :run and :poll are what
actually execute anything.`,
		Example: `local q = require("queue").new()
q:push(function(a) print(a) end, { args = { 42 }, priority = 5 })
print(q:poll())        -- runs what is due, returns the count
print(q:metrics().processed)`,
		SeeAlso: []string{"queue", "queue.channel"},
		Entries: []Entry{
			{Name: "push", Kind: EntryMethod, Signature: "q:push(fn [, opts]): string | nil, string",
				Summary: "Schedules fn and returns its job id, or nil plus an error.",
				Detail: `opts keys: priority (higher runs first), delay_ms, timeout_ms (a
deadline on STARTING — an expired job is dropped unrun), retries,
backoff_ms, args (an array passed to fn), payload and id.`},
			{Name: "run", Kind: EntryMethod, Signature: "q:run(): number",
				Summary: "Drains the queue, waiting out delays, and returns how many jobs ran. Blocks until empty or :stop()."},
			{Name: "poll", Kind: EntryMethod, Signature: "q:poll([max]): number",
				Summary: "Runs only the jobs already due, at most max of them, and returns the count. Never sleeps."},
			{Name: "stop", Kind: EntryMethod, Signature: "q:stop()",
				Summary: "Stops the queue; a blocked :run returns."},
			{Name: "is_stopped", Kind: EntryMethod, Signature: "q:is_stopped(): boolean", Summary: "Whether :stop has been called."},
			{Name: "size", Kind: EntryMethod, Signature: "q:size(): number", Summary: "The number of pending jobs. The # operator does the same."},
			{Name: "empty", Kind: EntryMethod, Signature: "q:empty(): boolean", Summary: "Whether nothing is pending."},
			{Name: "clear", Kind: EntryMethod, Signature: "q:clear()", Summary: "Discards every pending job."},
			{Name: "metrics", Kind: EntryMethod, Signature: "q:metrics(): table",
				Summary: "Counters and timings: enqueued, processed, succeeded, failed, retried, expired, dropped, pending, and the average and maximum wait and execution times in milliseconds."},
		},
	},
	{
		Name: "queue.channel", Kind: KindObject, RuntimeModule: "queue",
		Title:    "channel carrying Lua values",
		Synopsis: `local ch = require("queue").channel(4)`,
		Detail: `A Go channel with a Lua face, used to hand values back from the timer
goroutines queue.after and queue.tick create.

The data channel is never closed: :close closes a separate done channel,
so a send racing a close reports "closed" instead of panicking.`,
		SeeAlso: []string{"queue", "coroutine"},
		Entries: []Entry{
			{Name: "send", Kind: EntryMethod, Signature: "ch:send(v [, timeout_ms]): boolean, string?",
				Summary: "Sends a value, blocking until there is room or the timeout expires."},
			{Name: "receive", Kind: EntryMethod, Signature: "ch:receive([timeout_ms]): any, string?",
				Summary: "Receives a value, blocking until one arrives or the timeout expires."},
			{Name: "try_send", Kind: EntryMethod, Signature: "ch:try_send(v): boolean",
				Summary: "Sends only if it can be done immediately."},
			{Name: "try_receive", Kind: EntryMethod, Signature: "ch:try_receive(): any, boolean",
				Summary: "Receives only if a value is already waiting; the second return says whether one was."},
			{Name: "close", Kind: EntryMethod, Signature: "ch:close()",
				Summary: "Marks the channel closed. Later sends report an error rather than raising."},
			{Name: "is_closed", Kind: EntryMethod, Signature: "ch:is_closed(): boolean", Summary: "Whether :close has been called."},
			{Name: "len", Kind: EntryMethod, Signature: "ch:len(): number", Summary: "How many values are buffered. The # operator does the same."},
			{Name: "cap", Kind: EntryMethod, Signature: "ch:cap(): number", Summary: "The channel's buffer capacity."},
		},
	},
	{
		Name: "std.stack", Kind: KindObject, RuntimeModule: "std",
		Title: "LIFO stack", Synopsis: `local s = require("std").new_stack()`,
		SeeAlso: []string{"std", "std.queue", "std.deque"},
		Entries: []Entry{
			{Name: "push", Kind: EntryMethod, Signature: "s:push(v)", Summary: "Pushes a value onto the top."},
			{Name: "pop", Kind: EntryMethod, Signature: "s:pop(): any", Summary: "Removes and returns the top value, or nil when empty."},
			{Name: "peek", Kind: EntryMethod, Signature: "s:peek(): any", Summary: "Returns the top value without removing it."},
			{Name: "size", Kind: EntryMethod, Signature: "s:size(): number", Summary: "The number of values held."},
			{Name: "empty", Kind: EntryMethod, Signature: "s:empty(): boolean", Summary: "Whether the stack holds nothing."},
			{Name: "clear", Kind: EntryMethod, Signature: "s:clear()", Summary: "Removes every value."},
		},
	},
	{
		Name: "std.queue", Kind: KindObject, RuntimeModule: "std",
		Title: "FIFO queue", Synopsis: `local q = require("std").new_queue()`,
		Detail:  `Unrelated to the queue module, which schedules jobs.`,
		SeeAlso: []string{"std", "std.stack", "std.deque"},
		Entries: []Entry{
			{Name: "push", Kind: EntryMethod, Signature: "q:push(v)", Summary: "Appends a value to the back."},
			{Name: "pop", Kind: EntryMethod, Signature: "q:pop(): any", Summary: "Removes and returns the front value, or nil when empty."},
			{Name: "peek", Kind: EntryMethod, Signature: "q:peek(): any", Summary: "Returns the front value without removing it."},
			{Name: "size", Kind: EntryMethod, Signature: "q:size(): number", Summary: "The number of values held."},
			{Name: "empty", Kind: EntryMethod, Signature: "q:empty(): boolean", Summary: "Whether the queue holds nothing."},
			{Name: "clear", Kind: EntryMethod, Signature: "q:clear()", Summary: "Removes every value."},
		},
	},
	{
		Name: "std.deque", Kind: KindObject, RuntimeModule: "std",
		Title: "double-ended queue", Synopsis: `local d = require("std").new_deque()`,
		SeeAlso: []string{"std", "std.list"},
		Entries: []Entry{
			{Name: "push_front", Kind: EntryMethod, Signature: "d:push_front(v)", Summary: "Inserts a value at the front."},
			{Name: "push_back", Kind: EntryMethod, Signature: "d:push_back(v)", Summary: "Appends a value at the back."},
			{Name: "pop_front", Kind: EntryMethod, Signature: "d:pop_front(): any", Summary: "Removes and returns the front value."},
			{Name: "pop_back", Kind: EntryMethod, Signature: "d:pop_back(): any", Summary: "Removes and returns the back value."},
			{Name: "front", Kind: EntryMethod, Signature: "d:front(): any", Summary: "The front value, left in place."},
			{Name: "back", Kind: EntryMethod, Signature: "d:back(): any", Summary: "The back value, left in place."},
			{Name: "size", Kind: EntryMethod, Signature: "d:size(): number", Summary: "The number of values held."},
			{Name: "empty", Kind: EntryMethod, Signature: "d:empty(): boolean", Summary: "Whether the deque holds nothing."},
			{Name: "clear", Kind: EntryMethod, Signature: "d:clear()", Summary: "Removes every value."},
		},
	},
	{
		Name: "std.set", Kind: KindObject, RuntimeModule: "std",
		Title: "set of unique values", Synopsis: `local s = require("std").new_set()`,
		SeeAlso: []string{"std", "std.hashmap"},
		Entries: []Entry{
			{Name: "add", Kind: EntryMethod, Signature: "s:add(v): boolean", Summary: "Adds a value; false when it was already present."},
			{Name: "remove", Kind: EntryMethod, Signature: "s:remove(v): boolean", Summary: "Removes a value; false when it was absent."},
			{Name: "contains", Kind: EntryMethod, Signature: "s:contains(v): boolean", Summary: "Whether the value is a member."},
			{Name: "values", Kind: EntryMethod, Signature: "s:values(): table", Summary: "Every member, as an array. The order is unspecified."},
			{Name: "size", Kind: EntryMethod, Signature: "s:size(): number", Summary: "The number of members."},
			{Name: "empty", Kind: EntryMethod, Signature: "s:empty(): boolean", Summary: "Whether the set has no members."},
			{Name: "clear", Kind: EntryMethod, Signature: "s:clear()", Summary: "Removes every member."},
		},
	},
	{
		Name: "std.list", Kind: KindObject, RuntimeModule: "std",
		Title: "doubly linked list", Synopsis: `local l = require("std").new_list()`,
		SeeAlso: []string{"std", "std.deque"},
		Entries: []Entry{
			{Name: "push_front", Kind: EntryMethod, Signature: "l:push_front(v)", Summary: "Inserts a value at the head."},
			{Name: "push_back", Kind: EntryMethod, Signature: "l:push_back(v)", Summary: "Appends a value at the tail."},
			{Name: "pop_front", Kind: EntryMethod, Signature: "l:pop_front(): any", Summary: "Removes and returns the head value."},
			{Name: "pop_back", Kind: EntryMethod, Signature: "l:pop_back(): any", Summary: "Removes and returns the tail value."},
			{Name: "front", Kind: EntryMethod, Signature: "l:front(): any", Summary: "The head value, left in place."},
			{Name: "back", Kind: EntryMethod, Signature: "l:back(): any", Summary: "The tail value, left in place."},
			{Name: "to_array", Kind: EntryMethod, Signature: "l:to_array(): table", Summary: "Every value in order, as an ordinary array."},
			{Name: "size", Kind: EntryMethod, Signature: "l:size(): number", Summary: "The number of values held."},
			{Name: "empty", Kind: EntryMethod, Signature: "l:empty(): boolean", Summary: "Whether the list holds nothing."},
			{Name: "clear", Kind: EntryMethod, Signature: "l:clear()", Summary: "Removes every value."},
		},
	},
	{
		Name: "std.hashmap", Kind: KindObject, RuntimeModule: "std",
		Title: "key/value map", Synopsis: `local m = require("std").new_hashmap()`,
		SeeAlso: []string{"std", "std.set", "table"},
		Entries: []Entry{
			{Name: "put", Kind: EntryMethod, Signature: "m:put(k, v)", Summary: "Associates a key with a value, replacing any previous one."},
			{Name: "get", Kind: EntryMethod, Signature: "m:get(k): any", Summary: "The value stored under a key, or nil."},
			{Name: "contains", Kind: EntryMethod, Signature: "m:contains(k): boolean", Summary: "Whether the key is present."},
			{Name: "size", Kind: EntryMethod, Signature: "m:size(): number", Summary: "The number of pairs held."},
		},
	},
	{
		Name: "std.heap", Kind: KindObject, RuntimeModule: "std",
		Title: "binary heap", Synopsis: `local h = require("std").new_heap(function(a, b) return a < b end)`,
		Detail:  `The comparator decides priority: return true when a should come out before b.`,
		SeeAlso: []string{"std", "sort"},
		Entries: []Entry{
			{Name: "push", Kind: EntryMethod, Signature: "h:push(v)", Summary: "Inserts a value, restoring the heap order."},
			{Name: "pop", Kind: EntryMethod, Signature: "h:pop(): any", Summary: "Removes and returns the highest-priority value."},
			{Name: "top", Kind: EntryMethod, Signature: "h:top(): any", Summary: "The highest-priority value, left in place."},
			{Name: "size", Kind: EntryMethod, Signature: "h:size(): number", Summary: "The number of values held."},
			{Name: "empty", Kind: EntryMethod, Signature: "h:empty(): boolean", Summary: "Whether the heap holds nothing."},
		},
	},
	{
		Name: "std.trie", Kind: KindObject, RuntimeModule: "std",
		Title: "string prefix tree", Synopsis: `local t = require("std").new_trie()`,
		SeeAlso: []string{"std", "string"},
		Entries: []Entry{
			{Name: "insert", Kind: EntryMethod, Signature: "t:insert(...)", Summary: "Inserts one or more strings."},
			{Name: "find", Kind: EntryMethod, Signature: "t:find(s): boolean", Summary: "Whether the string was inserted."},
			{Name: "remove", Kind: EntryMethod, Signature: "t:remove(...)", Summary: "Removes one or more strings."},
			{Name: "compact", Kind: EntryMethod, Signature: "t:compact()", Summary: "Releases nodes left unused by removals."},
			{Name: "size", Kind: EntryMethod, Signature: "t:size(): number", Summary: "The number of strings stored."},
			{Name: "capacity", Kind: EntryMethod, Signature: "t:capacity(): number", Summary: "The number of nodes allocated."},
		},
	},
	{
		Name: "std.btree", Kind: KindObject, RuntimeModule: "std",
		Title: "ordered B-tree", Synopsis: `local b = require("std").new_btree(3)`,
		Detail:  `Keys may be numbers or strings; the type is fixed by the first insert and mixing afterwards raises.`,
		SeeAlso: []string{"std", "sort"},
		Entries: []Entry{
			{Name: "insert", Kind: EntryMethod, Signature: "b:insert(k [, v])", Summary: "Inserts a key, with an optional associated value."},
			{Name: "search", Kind: EntryMethod, Signature: "b:search(k): any", Summary: "The value stored under a key, or nil."},
			{Name: "remove", Kind: EntryMethod, Signature: "b:remove(k): any", Summary: "Removes a key and returns what it held."},
			{Name: "min", Kind: EntryMethod, Signature: "b:min(): any", Summary: "The smallest key."},
			{Name: "max", Kind: EntryMethod, Signature: "b:max(): any", Summary: "The largest key."},
			{Name: "size", Kind: EntryMethod, Signature: "b:size(): number", Summary: "The number of keys stored."},
			{Name: "empty", Kind: EntryMethod, Signature: "b:empty(): boolean", Summary: "Whether the tree holds nothing."},
		},
	},
	{
		Name: "plot.figure", Kind: KindObject, RuntimeModule: "plot",
		Title:    "SVG figure",
		Synopsis: `local fig = require("plot").figure()`,
		Detail: `Every method returns the figure, so calls chain. Axes re-range
automatically as series are added; a series given a label appears in the
legend.`,
		Example: `require("plot").figure()
  :scatter({1, 2, 3}, {1, 4, 9}, "squares")
  :size(640, 480):title("demo")
  :save("out.svg")`,
		SeeAlso: []string{"plot"},
		Entries: []Entry{
			{Name: "line", Kind: EntryMethod, Signature: "fig:line(xs, ys [, label]): figure", Summary: "Adds a line series."},
			{Name: "scatter", Kind: EntryMethod, Signature: "fig:scatter(xs, ys [, label]): figure", Summary: "Adds a scatter series."},
			{Name: "bar", Kind: EntryMethod, Signature: "fig:bar(labels, values [, label]): figure", Summary: "Adds a bar series."},
			{Name: "histogram", Kind: EntryMethod, Signature: "fig:histogram(values [, bins [, label]]): figure",
				Summary: "Adds a histogram of values."},
			{Name: "title", Kind: EntryMethod, Signature: "fig:title(s): figure", Summary: "Sets the chart title."},
			{Name: "xlabel", Kind: EntryMethod, Signature: "fig:xlabel(s): figure", Summary: "Sets the x-axis label."},
			{Name: "ylabel", Kind: EntryMethod, Signature: "fig:ylabel(s): figure", Summary: "Sets the y-axis label."},
			{Name: "size", Kind: EntryMethod, Signature: "fig:size(w, h): figure", Summary: "Sets the output size in pixels."},
			{Name: "to_svg", Kind: EntryMethod, Signature: "fig:to_svg(): string", Summary: "Renders the figure and returns the SVG text."},
			{Name: "save", Kind: EntryMethod, Signature: "fig:save(path): figure", Summary: "Renders the figure and writes it to a file."},
		},
	},
	{
		Name: "ndarray.array", Kind: KindObject, RuntimeModule: "ndarray",
		Title:    "N-dimensional array instance",
		Synopsis: `local a = require("ndarray").array({{1, 2}, {3, 4}})`,
		Detail: `Arithmetic operators are overloaded (+ - * / % ^ and unary -) and
broadcast, so a + b works elementwise and a * 2 scales. == compares
shape and contents. # gives the length of the first axis.

Reductions take an optional 1-based axis; with no axis they reduce the
whole array and return a plain number.`,
		Example: `local nd = require("ndarray")
local a = nd.arange(6):reshape({2, 3})
print(a:sum(1))        -- column sums
print(a:transpose())
print((a * 2):max())`,
		SeeAlso: []string{"ndarray", "linalg"},
		Entries: []Entry{
			{Name: "get", Kind: EntryMethod, Signature: "a:get(...): number", Summary: "The element at the given 1-based indices."},
			{Name: "set", Kind: EntryMethod, Signature: "a:set(value, ...)", Summary: "Writes an element at the given 1-based indices."},
			{Name: "shape", Kind: EntryMethod, Signature: "a:shape(): table", Summary: "The size of each dimension, as an array."},
			{Name: "ndim", Kind: EntryMethod, Signature: "a:ndim(): number", Summary: "The number of dimensions."},
			{Name: "size", Kind: EntryMethod, Signature: "a:size(): number", Summary: "The total number of elements."},
			{Name: "reshape", Kind: EntryMethod, Signature: "a:reshape(shape): ndarray",
				Summary: "A view of the same data with a new shape. The element count must match."},
			{Name: "flatten", Kind: EntryMethod, Signature: "a:flatten(): ndarray", Summary: "A 1-D copy of the array."},
			{Name: "transpose", Kind: EntryMethod, Signature: "a:transpose(): ndarray", Summary: "The array with its axes reversed."},
			{Name: "copy", Kind: EntryMethod, Signature: "a:copy(): ndarray", Summary: "An independent copy."},
			{Name: "to_table", Kind: EntryMethod, Signature: "a:to_table(): table", Summary: "A nested Lua table with the same contents."},
			{Name: "tolist", Kind: EntryMethod, Signature: "a:tolist(): table", Summary: "A nested Lua table — the NumPy spelling of to_table."},
			{Name: "add", Kind: EntryMethod, Signature: "a:add(b): ndarray", Summary: "Elementwise sum, broadcasting. The + operator does the same."},
			{Name: "sub", Kind: EntryMethod, Signature: "a:sub(b): ndarray", Summary: "Elementwise difference, broadcasting."},
			{Name: "mul", Kind: EntryMethod, Signature: "a:mul(b): ndarray", Summary: "Elementwise product, broadcasting."},
			{Name: "div", Kind: EntryMethod, Signature: "a:div(b): ndarray", Summary: "Elementwise quotient, broadcasting."},
			{Name: "neg", Kind: EntryMethod, Signature: "a:neg(): ndarray", Summary: "Elementwise negation."},
			{Name: "pow", Kind: EntryMethod, Signature: "a:pow(e): ndarray", Summary: "Raises every element to the power e."},
			{Name: "matmul", Kind: EntryMethod, Signature: "a:matmul(b): ndarray | number", Summary: "Matrix product with b."},
			{Name: "dot", Kind: EntryMethod, Signature: "a:dot(b): ndarray | number",
				Summary: "Dot product. Two vectors give a plain number."},
			{Name: "sum", Kind: EntryMethod, Signature: "a:sum([axis]): ndarray | number", Summary: "Sum over an axis, or of everything."},
			{Name: "mean", Kind: EntryMethod, Signature: "a:mean([axis]): ndarray | number", Summary: "Mean over an axis, or of everything."},
			{Name: "prod", Kind: EntryMethod, Signature: "a:prod([axis]): ndarray | number", Summary: "Product over an axis, or of everything."},
			{Name: "min", Kind: EntryMethod, Signature: "a:min([axis]): ndarray | number", Summary: "Minimum over an axis, or of everything."},
			{Name: "max", Kind: EntryMethod, Signature: "a:max([axis]): ndarray | number", Summary: "Maximum over an axis, or of everything."},
			{Name: "std", Kind: EntryMethod, Signature: "a:std([axis]): ndarray | number", Summary: "Standard deviation over an axis, or of everything."},
			{Name: "var", Kind: EntryMethod, Signature: "a:var([axis]): ndarray | number", Summary: "Variance over an axis, or of everything."},
			{Name: "argmin", Kind: EntryMethod, Signature: "a:argmin([axis]): ndarray | number", Summary: "Index of the smallest element."},
			{Name: "argmax", Kind: EntryMethod, Signature: "a:argmax([axis]): ndarray | number", Summary: "Index of the largest element."},
			{Name: "abs", Kind: EntryMethod, Signature: "a:abs(): ndarray", Summary: "Elementwise absolute value."},
			{Name: "sqrt", Kind: EntryMethod, Signature: "a:sqrt(): ndarray", Summary: "Elementwise square root."},
			{Name: "exp", Kind: EntryMethod, Signature: "a:exp(): ndarray", Summary: "Elementwise exponential."},
			{Name: "log", Kind: EntryMethod, Signature: "a:log(): ndarray", Summary: "Elementwise natural logarithm."},
			{Name: "sin", Kind: EntryMethod, Signature: "a:sin(): ndarray", Summary: "Elementwise sine."},
			{Name: "cos", Kind: EntryMethod, Signature: "a:cos(): ndarray", Summary: "Elementwise cosine."},
			{Name: "tanh", Kind: EntryMethod, Signature: "a:tanh(): ndarray", Summary: "Elementwise hyperbolic tangent."},
			{Name: "floor", Kind: EntryMethod, Signature: "a:floor(): ndarray", Summary: "Elementwise floor."},
			{Name: "ceil", Kind: EntryMethod, Signature: "a:ceil(): ndarray", Summary: "Elementwise ceiling."},
			{Name: "clip", Kind: EntryMethod, Signature: "a:clip(lo, hi): ndarray", Summary: "Constrains every element to [lo, hi]."},
			{Name: "map", Kind: EntryMethod, Signature: "a:map(fn): ndarray", Summary: "Applies a Lua function to every element and returns the result."},
			{Name: "show", Kind: EntryMethod, Signature: "a:show()", Summary: "Prints the array in the NumPy-style layout __tostring produces."},
		},
	},
	{
		Name: "dataframe.frame", Kind: KindObject, RuntimeModule: "dataframe",
		Title:    "data frame instance",
		Synopsis: `local df = require("dataframe").new({ a = {1, 2} })`,
		Detail: `Operations return new frames rather than mutating, so they chain.
Row callbacks receive a table keyed by column name.`,
		Example: `local df = require("dataframe").from_csv("people.csv")
print(df:select("name", "age")
        :filter(function(r) return r.age > 30 end)
        :sort("age"))`,
		SeeAlso: []string{"dataframe", "csv", "stats"},
		Entries: []Entry{
			{Name: "col", Kind: EntryMethod, Signature: "df:col(name): table", Summary: "One column, as an array."},
			{Name: "row", Kind: EntryMethod, Signature: "df:row(i): table", Summary: "One row, as a table keyed by column name."},
			{Name: "select", Kind: EntryMethod, Signature: "df:select(...): frame", Summary: "A frame with only the named columns."},
			{Name: "drop", Kind: EntryMethod, Signature: "df:drop(...): frame", Summary: "A frame without the named columns."},
			{Name: "rename", Kind: EntryMethod, Signature: "df:rename(mapping): frame",
				Summary: "A frame with columns renamed, given an old-name to new-name table."},
			{Name: "filter", Kind: EntryMethod, Signature: "df:filter(fn): frame",
				Summary: "The rows for which fn(row) is truthy."},
			{Name: "with_column", Kind: EntryMethod, Signature: "df:with_column(name, fn): frame",
				Summary: "A frame with an extra column, each value computed by fn(row)."},
			{Name: "sort", Kind: EntryMethod, Signature: "df:sort(name [, descending]): frame",
				Summary: "The rows ordered by one column."},
			{Name: "group_by", Kind: EntryMethod, Signature: "df:group_by(name, aggs): frame",
				Summary: "Groups by a column and aggregates the rest, given a column-to-aggregation table."},
			{Name: "head", Kind: EntryMethod, Signature: "df:head([n]): frame", Summary: "The first n rows (default 5)."},
			{Name: "tail", Kind: EntryMethod, Signature: "df:tail([n]): frame", Summary: "The last n rows (default 5)."},
			{Name: "shape", Kind: EntryMethod, Signature: "df:shape(): number, number", Summary: "The row and column counts."},
			{Name: "columns", Kind: EntryMethod, Signature: "df:columns(): table", Summary: "The column names, in order."},
			{Name: "to_csv", Kind: EntryMethod, Signature: "df:to_csv(path)", Summary: "Writes the frame to a CSV file."},
		},
	},
	{
		Name: "ml.net", Kind: KindObject, RuntimeModule: "ml",
		Title:    "neural network instance",
		Synopsis: `local net = require("ml").new(config)`,
		SeeAlso:  []string{"ml", "classification"},
		Entries: []Entry{
			{Name: "train", Kind: EntryMethod, Signature: "net:train(data): number",
				Summary: "Trains on an array of {input, output} pairs and returns the final loss."},
			{Name: "predict", Kind: EntryMethod, Signature: "net:predict(input): table",
				Summary: "Runs a forward pass and returns the output layer."},
			{Name: "save", Kind: EntryMethod, Signature: "net:save(): string",
				Summary: "Serialises the network to a string, restorable with ml.load."},
			{Name: "save_file", Kind: EntryMethod, Signature: "net:save_file(path)",
				Summary: "Writes the serialised network to a file, restorable with ml.load_file."},
		},
	},
}
