require 'optparse'
require 'uri'
require 'logger'
require 'faraday'
require 'elasticsearch'

class Post
  attr_reader :url, :n, :name, :mail, :meta, :body, :title

  def initialize(url, n, line)
    @url, @n = url, n
    @name, @mail, @meta, @body, @title = line.encode('UTF-8', 'cp932', invalid: :replace).chomp.split /<>/
  end

  def id
    @id ||= begin
      u = URI.parse(@url)
      [ u.host, *u.path.split('/')[1..-1] ].join('-') + ':' + n.to_s
    end
  end
end

etch_origin = ARGV.shift || 'http://localhost:25252'
es_origin   = ARGV.shift || 'http://localhost:9200'

logger = Logger.new(STDERR)
logger.level = Logger::INFO

etch = Faraday.new url: etch_origin
es   = Elasticsearch::Client.new url: es_origin, logger: logger

etch.get('/').body.each_line do |url|
  url.chomp!

  logger.info("#{url}...")

  posts = etch.get('/cache', { url: url }).body.each_line.with_index.map do |post, i|
    begin
      Post.new(url, i+1, post)
    rescue => e
      logger.error "#{url} at #{i+1}: #{e}"
      nil
    end
  end.compact

  begin
    es.bulk body: posts.map { |p|
      { index: { _index: 'etch', _type: 'post', _id: p.id, data: { name: p.name, mail: p.mail, meta: p.meta, body_html: p.body, title: p.title } } }
    } .to_a
  rescue => e
    logger.error "#{url}: #{e}"
  end
end
