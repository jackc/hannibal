begin
  require "bundler"
  Bundler.setup
rescue LoadError
  puts "You must `gem install bundler` and `bundle install` to run rake tasks"
end

require "rake/clean"
require "fileutils"
require "rake/testtask"
require "erb"

CLOBBER.include("tmp")

def passthrough_args
  i = ARGV.index("--")
  args = i ? ARGV[(i+1)..-1].join(' ') : nil
  unless args
    puts 'No arguments received. Pass arguments after "--"'
    exit 1
  end
  args
end

directory "tmp/development/bin"

file "embed/statik/statik.go" => FileList["embed/root/**/*"] do
  sh "statik -src embed/root -dest embed"
end

file "tmp/development/bin/hannibal" => ["embed/statik/statik.go", *FileList["**/*.go"]] do |t|
  sh "go build -o tmp/development/bin/hannibal"
end

namespace :build do
  desc "Build backend"
  task backend: ["tmp/development/bin/hannibal"]
end

desc "Build backend and frontend"
task build: ["build:backend"]

desc "Run hannibal"
task run: ["build:backend"] do
  exec "tmp/development/bin/hannibal #{passthrough_args}"
end

desc "Watch for source changes, rebuild and rerun"
task :rerun do
  exec "react2fs -include='\.(go|sql)$' -exclude='^tmp|^\.' rake run -- #{passthrough_args}"
end

desc "Run backend tests"
task "test:backend" do
  sh "go test ./... -count=1"
end
