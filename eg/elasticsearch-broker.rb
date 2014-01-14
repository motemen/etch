require 'optparse'
require 'uri'
require 'logger'
require 'faraday'
require 'elasticsearch'
require 'yajl'
require 'yajl/http_stream'

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

class Broker
  def initialize(etch_origin: 'http://localhost:25252', es_origin: 'http://localhost:9200')
    @logger = Logger.new(STDERR)
    @logger.level = Logger::DEBUG

    @etch_origin = etch_origin
    @es_origin   = es_origin

    @etch = Faraday.new url: etch_origin
    @es   = Elasticsearch::Client.new url: es_origin, logger: @logger
  end

  def index_all_urls!
    @logger.info('fetching all thread urls...')
    @etch.get('/').body.each_line do |url|
      url.chomp!
      index_thread_posts!(url)
    end
  end

  def index_delta_urls_streaming!
    @logger.info('waiting for delta updates...')

    Yajl::HttpStream.get(@etch.url_prefix + 'events', symbolize_keys: true) do |event|
      @logger.debug("got event: #{event}")

      # keys: event, since, url
      case event[:event]
      when 'cacheUpdate'
        index_thread_posts!(event[:url]) # TODO use 'since'
      else
        @logger.warn("unknown event: #{event[:event]}")
      end
    end
  end

  def index_thread_posts!(url)
    @logger.info("indexing #{url}...")

    posts = nil

    5.times do
      res = @etch.get('/cache', { url: url })

      if res.status != 200
        @logger.warn "retrieving cached data of #{url}: got #{res.status}; retry..."
        sleep 0.5
        next
      end

      posts = res.body.each_line.with_index.map do |post, i|
        begin
          Post.new(url, i+1, post)
        rescue => e
          @logger.error "#{url} at #{i+1}: #{e}"
          nil
        end
      end.compact

      break
    end

    begin
      @es.bulk body: posts.map { |p|
        { index: { _index: 'etch', _type: 'post', _id: p.id, data: { name: p.name, mail: p.mail, meta: p.meta, body_html: p.body, title: p.title } } }
      } .to_a
    rescue => e
      @logger.error "#{url}: #{e}"
    end
  end
end

config = {}
do_index_all = nil
do_index_delta = nil

OptionParser.new do |opts|
  opts.on('--etch http://localhost:25252', 'etch origin') do |o|
    config[:etch_origin] = o
  end

  opts.on('--es http://localhost:9200', 'ElasticSearch origin') do |o|
    config[:es_origin] = o
  end

  opts.on('--all')   { do_index_all   = true }
  opts.on('--delta') { do_index_delta = true }
end.parse!

if do_index_all == nil && do_index_delta == nil
  # asuume both are specified
  do_index_all = do_index_delta = true
end

broker = Broker.new(config)
broker.index_all_urls! if do_index_all
broker.index_delta_urls_streaming! if do_index_delta
