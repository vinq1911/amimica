class UserService
  def initialize(repo)
    @repo = repo
  end

  def find_user(id)
    user = @repo.find(id)
    raise NotFoundError, "User not found: #{id}" unless user
    raise InactiveError, "User is inactive" unless user.active?
    user
  end

  def update_user(id, attrs)
    user = @repo.find(id)
    raise NotFoundError, "User not found: #{id}" unless user
    raise InactiveError, "User is inactive" unless user.active?
    user.update(attrs)
    @repo.save(user)
    user
  end

  def delete_user(id)
    user = @repo.find(id)
    raise NotFoundError, "User not found: #{id}" unless user
    raise InactiveError, "User is inactive" unless user.active?
    @repo.delete(user)
  end
end
