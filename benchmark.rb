#!/usr/bin/env ruby
# frozen_string_literal: true

require "open3"
require "optparse"

# bench.rb — a small benchmark harness for the luascript interpreter.
#
# Runs each script in a directory `warmup + samples` times, discards the
# warmup runs, and reports min / median / mean / stddev. Min is usually the
# most meaningful single number for a VM: it's the run least disturbed by the
# OS scheduler, GC, or CPU frequency scaling.
#
# Two clocks:
#   --mode wall  (default) wall-clock incl. process spawn + Go runtime init
#   --mode vm    parses the interpreter's own `--time` output (compile+run,
#                no process-spawn overhead)
#
# Usage:
#   ruby bench.rb                                   # ./benchmarks/*.lsc, wall clock
#   ruby bench.rb --mode vm                         # VM-internal time only
#   ruby bench.rb --bin ./luascript --dir benchmarks --samples 30 --warmup 5
#   ruby bench.rb --compare                         # fib.lsc vs fib.lua/.rb/.py side by side

CONFIG = {
  bin:     ENV.fetch("LUASCRIPT_BIN", "./luascript"),
  dir:     "benchmarks",
  ext:     ".lsc",
  samples: 20,
  warmup:  3,
  mode:    :wall,
  compare: false,
}.freeze

opts = CONFIG.dup
OptionParser.new do |o|
  o.banner = "usage: ruby bench.rb [options]"
  o.on("--bin PATH")        { |v| opts[:bin] = v }
  o.on("--dir PATH")        { |v| opts[:dir] = v }
  o.on("--ext EXT")         { |v| opts[:ext] = v.start_with?(".") ? v : ".#{v}" }
  o.on("--samples N", Integer) { |v| opts[:samples] = v }
  o.on("--warmup N",  Integer) { |v| opts[:warmup]  = v }
  o.on("--mode MODE", %i[wall vm]) { |v| opts[:mode] = v }
  o.on("--compare")         { opts[:compare] = true }
  o.on("-h", "--help") { puts o; exit 0 }
end.parse!

GO_UNITS = {
  "ns" => 1e-9, "us" => 1e-6, "µs" => 1e-6, "μs" => 1e-6,
  "ms" => 1e-3, "s" => 1.0, "m" => 60.0, "h" => 3600.0,
}.freeze

def parse_go_duration(str)
  s = str.strip
  return 0.0 if s == "0s"
  total = 0.0
  matched = false
  # Multi-char units listed before single-char so "ms" wins over "m"+"s".
  while (m = s.match(/\A(\d+(?:\.\d+)?)(ns|µs|μs|us|ms|s|m|h)/))
    total += m[1].to_f * GO_UNITS[m[2]]
    s = s[m[0].length..]
    matched = true
  end
  matched && s.empty? ? total : nil
end

def time_cmd(cmd, parse_vm:)
  t0 = Process.clock_gettime(Process::CLOCK_MONOTONIC)
  out, err, status = Open3.capture3(*cmd)
  t1 = Process.clock_gettime(Process::CLOCK_MONOTONIC)

  unless status.success?
    msg = (err.empty? ? out : err).strip
    raise "#{cmd.join(' ')} failed (exit #{status.exitstatus}): #{msg}"
  end

  return t1 - t0 unless parse_vm

  line = out.lines.reverse.find { |l| l.include?("Execution time:") }
  raise "no 'Execution time:' line — did you build with --time support?" unless line
  secs = parse_go_duration(line.split("Execution time:").last)
  raise "could not parse duration: #{line.strip.inspect}" unless secs
  secs
end

def bench(cmd, samples:, warmup:, parse_vm:)
  warmup.times { time_cmd(cmd, parse_vm: parse_vm) }
  data = Array.new(samples) { time_cmd(cmd, parse_vm: parse_vm) }
  stats(data)
end

def stats(xs)
  sorted = xs.sort
  n = sorted.length
  mean = xs.sum / n
  median = n.odd? ? sorted[n / 2] : (sorted[n / 2 - 1] + sorted[n / 2]) / 2.0
  var = xs.sum { |x| (x - mean)**2 } / n
  { min: sorted.first, median: median, mean: mean, stddev: Math.sqrt(var), n: n }
end

def fmt(secs)
  if    secs >= 1    then format("%8.3f s",  secs)
  elsif secs >= 1e-3 then format("%8.3f ms", secs * 1e3)
  elsif secs >= 1e-6 then format("%8.1f µs", secs * 1e6)
  else                    format("%8.0f ns", secs * 1e9)
  end
end

RUNNERS = {
  ".lua" => ->(f) { ["lua", f] },
  ".rb"  => ->(f) { ["ruby", f] },
  ".py"  => ->(f) { ["python3", f] },
}.freeze

def runner_for(ext, bin)
  return ->(f) { [bin, f] } if ext == CONFIG[:ext]
  RUNNERS[ext]
end

abort "interpreter not found: #{opts[:bin]}" unless File.exist?(opts[:bin]) || opts[:bin] !~ %r{/}
abort "no such dir: #{opts[:dir]}" unless Dir.exist?(opts[:dir])

if opts[:compare]
  # Group files by basename: fib.lsc, fib.lua, fib.rb -> one "fib" row group.
  exts = [opts[:ext], *RUNNERS.keys]
  groups = Dir.glob(File.join(opts[:dir], "*")).group_by { |f| File.basename(f, ".*") }
  groups = groups.sort.to_h

  puts "comparison · wall clock · #{opts[:samples]} samples, #{opts[:warmup]} warmup\n\n"
  groups.each do |name, files|
    puts name
    files.sort.each do |file|
      ext = File.extname(file)
      run = runner_for(ext, opts[:bin])
      next unless run
      begin
        s = bench(run.call(file), samples: opts[:samples], warmup: opts[:warmup], parse_vm: false)
        printf("  %-8s  min %s   median %s   ±%s\n",
               ext, fmt(s[:min]), fmt(s[:median]), fmt(s[:stddev]))
      rescue => e
        printf("  %-8s  ERROR: %s\n", ext, e.message)
      end
    end
    puts
  end
else
  files = Dir.glob(File.join(opts[:dir], "*#{opts[:ext]}")).sort
  abort "no #{opts[:ext]} files in #{opts[:dir]}" if files.empty?

  parse_vm = opts[:mode] == :vm
  puts "luascript bench · #{opts[:mode]} clock · #{opts[:samples]} samples, #{opts[:warmup]} warmup\n\n"
  printf("%-24s %10s %10s %10s %10s\n", "script", "min", "median", "mean", "stddev")
  puts "-" * 68
  files.each do |file|
    cmd = parse_vm ? [opts[:bin], "--time", file] : [opts[:bin], file]
    begin
      s = bench(cmd, samples: opts[:samples], warmup: opts[:warmup], parse_vm: parse_vm)
      printf("%-24s %s %s %s %s\n", File.basename(file),
             fmt(s[:min]), fmt(s[:median]), fmt(s[:mean]), fmt(s[:stddev]))
    rescue => e
      printf("%-24s ERROR: %s\n", File.basename(file), e.message)
    end
  end
end
