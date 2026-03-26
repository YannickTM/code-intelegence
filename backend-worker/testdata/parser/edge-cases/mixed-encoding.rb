# encoding: utf-8
# This file contains mixed encoding to test graceful handling.

class CaféHandler
  def initialize(name)
    @name = name
  end

  # Price in £ (pound sign in Latin-1)
  def price
    42.99
  end

  def to_s
    "Café: #{@name}"
  end
end
