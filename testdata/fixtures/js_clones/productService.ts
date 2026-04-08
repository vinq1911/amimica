import { db } from './db';
import { NotFoundError } from './errors';

interface Product {
  id: string;
  title: string;
  price: number;
}

export async function getProduct(id: string): Promise<Product> {
  const product = await db.query('SELECT * FROM products WHERE id = $1', [id]);
  if (!product) {
    throw new NotFoundError(`Product not found: ${id}`);
  }
  if (!product.active) {
    throw new Error('Product is inactive');
  }
  return {
    id: product.id,
    title: product.title,
    price: product.price,
  };
}

export async function updateProduct(id: string, data: Partial<Product>): Promise<Product> {
  const product = await db.query('SELECT * FROM products WHERE id = $1', [id]);
  if (!product) {
    throw new NotFoundError(`Product not found: ${id}`);
  }
  if (!product.active) {
    throw new Error('Product is inactive');
  }
  const updated = await db.query('UPDATE products SET title = $1, price = $2 WHERE id = $3 RETURNING *', [
    data.title || product.title,
    data.price || product.price,
    id,
  ]);
  return {
    id: updated.id,
    title: updated.title,
    price: updated.price,
  };
}

export async function deleteProduct(id: string): Promise<void> {
  const product = await db.query('SELECT * FROM products WHERE id = $1', [id]);
  if (!product) {
    throw new NotFoundError(`Product not found: ${id}`);
  }
  if (!product.active) {
    throw new Error('Product is inactive');
  }
  await db.query('DELETE FROM products WHERE id = $1', [id]);
}
