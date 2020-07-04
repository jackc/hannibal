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

directory "tmp/development/bin"

file "db/statik/statik.go" => FileList["db/foobarbuilder_migrations/*"] do
  sh "statik -src db/foobarbuilder_migrations -dest db"
end

file "tmp/development/bin/foobarbuilder" => ["db/statik/statik.go", *FileList["**/*.go"]] do |t|
  sh "go build -o tmp/development/bin/foobarbuilder"
end

file "tmp/development/.sql-installed" => "postgresql/setup.sql" do |t|
  sh "psql -f postgresql/setup.sql"
  sh "touch tmp/development/.sql-installed"
end


namespace :build do
  desc "Build backend"
  task backend: ["tmp/development/bin/foobarbuilder"]

  desc "Install SQL"
  task installsql: ["tmp/development/.sql-installed"]
end

desc "Build backend and frontend"
task build: ["build:backend"]

desc "Run foobarbuilder"
task run: ["build:installsql", "build:backend"] do
  exec "tmp/development/bin/foobarbuilder serve"
end

desc "Watch for source changes and rebuild and rerun"
task :rerun do
  exec "react2fs -include='\.(go|sql)$' rake run"
end

desc "Run backend tests"
task "test:backend" do
  sh "go test ./... -count=1"
end
