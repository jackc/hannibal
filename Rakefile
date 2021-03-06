require "rake/clean"
require "fileutils"
require "rake/testtask"
require "erb"

CLOBBER.include("tmp", "srvman/tmp")

def passthrough_args
  i = ARGV.index("--")
  args = i ? ARGV[(i+1)..-1].join(' ') : nil
  unless args
    puts 'No arguments received. Pass arguments after "--"'
    exit 1
  end
  args
end

directory "srvman/tmp/test/bin"
directory "tmp/development/bin"
directory "tmp/test/bin"

file "embed/statik/statik.go" => FileList["embed/root/**/*"] do
  sh "statik -src embed/root -dest embed"
end

file "tmp/development/bin/hannibal" => ["embed/statik/statik.go", *FileList["**/*.go"]].reject { |f| f =~ /_test.go$/ || f =~ /testdata/} do |t|
  sh "go build -o tmp/development/bin/hannibal"
end

desc "Build"
task build: ["tmp/development/bin/hannibal"]

desc "Run hannibal"
task run: ["build"] do
  exec "tmp/development/bin/hannibal #{passthrough_args}"
end

desc "Watch for source changes, rebuild and rerun"
task :rerun do
  exec "react2fs -include='\\.(go|sql)$' -exclude='^tmp|^\\.' rake run -- #{passthrough_args}"
end

file "tmp/test/bin/hannibal" => ["tmp/test/bin", "tmp/development/bin/hannibal"] do |t|
  FileUtils.copy_file "tmp/development/bin/hannibal", "tmp/test/bin/hannibal"
end

file "tmp/test/bin/http_server" => ["tmp/test/bin", *FileList["testdata/http_server/**/*.go"]] do
  sh "go build -o tmp/test/bin/http_server github.com/jackc/hannibal/testdata/http_server"
end

file "srvman/tmp/test/bin/http_server" => ["srvman/tmp/test/bin", *FileList["srvman/testdata/http_server/**/*.go"]] do
  sh "go build -o srvman/tmp/test/bin/http_server github.com/jackc/hannibal/srvman/testdata/http_server"
end

desc "Build test dependencies"
task testdep: ["tmp/test/bin/hannibal", "srvman/tmp/test/bin/http_server", "tmp/test/bin/http_server"]

desc "Run tests"
task test: :testdep do
  sh "go test ./..."
end

task default: :test
