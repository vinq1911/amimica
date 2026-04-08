import { db } from './db';
import { NotFoundError } from './errors';

interface User {
  id: string;
  name: string;
  email: string;
}

export async function getUser(id: string): Promise<User> {
  const user = await db.query('SELECT * FROM users WHERE id = $1', [id]);
  if (!user) {
    throw new NotFoundError(`User not found: ${id}`);
  }
  if (!user.active) {
    throw new Error('User is inactive');
  }
  return {
    id: user.id,
    name: user.name,
    email: user.email,
  };
}

export async function updateUser(id: string, data: Partial<User>): Promise<User> {
  const user = await db.query('SELECT * FROM users WHERE id = $1', [id]);
  if (!user) {
    throw new NotFoundError(`User not found: ${id}`);
  }
  if (!user.active) {
    throw new Error('User is inactive');
  }
  const updated = await db.query('UPDATE users SET name = $1, email = $2 WHERE id = $3 RETURNING *', [
    data.name || user.name,
    data.email || user.email,
    id,
  ]);
  return {
    id: updated.id,
    name: updated.name,
    email: updated.email,
  };
}

export async function deleteUser(id: string): Promise<void> {
  const user = await db.query('SELECT * FROM users WHERE id = $1', [id]);
  if (!user) {
    throw new NotFoundError(`User not found: ${id}`);
  }
  if (!user.active) {
    throw new Error('User is inactive');
  }
  await db.query('DELETE FROM users WHERE id = $1', [id]);
}
