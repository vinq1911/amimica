class ProductService
  def initialize(repo)
    @repo = repo
  end

  def find_product(id)
    product = @repo.find(id)
    raise NotFoundError, "Product not found: #{id}" unless product
    raise InactiveError, "Product is inactive" unless product.active?
    product
  end

  def update_product(id, attrs)
    product = @repo.find(id)
    raise NotFoundError, "Product not found: #{id}" unless product
    raise InactiveError, "Product is inactive" unless product.active?
    product.update(attrs)
    @repo.save(product)
    product
  end

  def delete_product(id)
    product = @repo.find(id)
    raise NotFoundError, "Product not found: #{id}" unless product
    raise InactiveError, "Product is inactive" unless product.active?
    @repo.delete(product)
  end
end
