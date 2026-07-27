const CART_KEY = 'qzq_cart';
const selectedSizes = {};

function getCart() {
  try {
    return JSON.parse(localStorage.getItem(CART_KEY)) || { items: [] };
  } catch {
    return { items: [] };
  }
}

function saveCart(cart) {
  cart.updated_at = new Date().toISOString();
  localStorage.setItem(CART_KEY, JSON.stringify(cart));
  updateCartCount();
}

function addToCart(productId, slug, name, size, price) {
  const cart = getCart();
  const existing = cart.items.find(i => i.product_id === productId && i.size === size);
  if (existing) {
    existing.quantity += 1;
  } else {
    cart.items.push({ product_id: productId, product_slug: slug, product_name: name, size, quantity: 1, price });
  }
  saveCart(cart);
}

function addToCartFromPage(productId, slug, name, price) {
  const size = selectedSizes[slug] || 'M';
  addToCart(productId, slug, name, size, price);
  flashCartBadge();
}

function selectSize(slug, size, btn) {
  selectedSizes[slug] = size;
  const container = document.getElementById('sizes-' + slug);
  if (!container) return;
  container.querySelectorAll('.sz').forEach(b => b.classList.remove('sz--active'));
  btn.classList.add('sz--active');
}

function setActiveSide(btn) {
  const switcher = btn.closest('.photo-switcher');
  if (!switcher) return;
  switcher.querySelectorAll('.side-btn').forEach(b => b.classList.remove('side-btn--active'));
  btn.classList.add('side-btn--active');
}

function updateCartCount() {
  const cart = getCart();
  const count = cart.items.reduce((sum, i) => sum + i.quantity, 0);
  document.querySelectorAll('[data-cart-count]').forEach(el => el.textContent = count);
}

function flashCartBadge() {
  const badge = document.querySelector('.cart-badge');
  if (!badge) return;
  badge.style.background = 'var(--red)';
  badge.style.color = '#fff';
  setTimeout(() => { badge.style.background = ''; badge.style.color = ''; }, 600);
}

function removeCartItem(productId, size) {
  var cart = getCart();
  cart.items = cart.items.filter(function(i) {
    return !(i.product_id === productId && i.size === size);
  });
  saveCart(cart);
  if (typeof renderCartPage === 'function') renderCartPage();
}

// ── Size guide modal ──
function openSizeModal() {
  document.getElementById('size-modal').classList.add('open');
}
function closeSizeModal(e) {
  if (e.target.id === 'size-modal') {
    document.getElementById('size-modal').classList.remove('open');
  }
}

// ── Lightbox ──
let lbImages = [];
let lbLabels = [];
let lbIndex  = 0;

function openLightbox(slug, currentSrc) {
  const half = document.querySelector(`.half[data-slug="${slug}"]`);
  if (!half) return;

  const front = half.dataset.front;
  const back  = half.dataset.back;

  lbImages = [front, back].filter(Boolean);
  lbLabels = ['front', 'back'];
  lbIndex  = lbImages.indexOf(currentSrc);
  if (lbIndex === -1) lbIndex = 0;

  renderLightbox();
  document.getElementById('lightbox').classList.add('open');
  document.body.style.overflow = 'hidden';
}

function renderLightbox() {
  document.getElementById('lightbox-img').src = lbImages[lbIndex];

  const counter = document.getElementById('lb-counter');
  counter.textContent = lbImages.length > 1
    ? lbLabels[lbIndex] + '  ·  ' + (lbIndex + 1) + ' / ' + lbImages.length
    : '';

  const showNav = lbImages.length > 1;
  document.querySelectorAll('.lb-nav').forEach(b => {
    b.style.display = showNav ? 'block' : 'none';
  });
}

function lbPrev() {
  lbIndex = (lbIndex - 1 + lbImages.length) % lbImages.length;
  renderLightbox();
}

function lbNext() {
  lbIndex = (lbIndex + 1) % lbImages.length;
  renderLightbox();
}

function closeLightbox() {
  document.getElementById('lightbox').classList.remove('open');
  document.body.style.overflow = '';
  lbImages = [];
}

document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    closeLightbox();
    document.getElementById('size-modal')?.classList.remove('open');
  }
  if (!document.getElementById('lightbox')?.classList.contains('open')) return;
  if (e.key === 'ArrowLeft')  lbPrev();
  if (e.key === 'ArrowRight') lbNext();
});

document.addEventListener('DOMContentLoaded', () => {
  updateCartCount();
  document.querySelectorAll('.sz--active').forEach(btn => {
    const slug = btn.dataset.slug;
    const size = btn.dataset.size;
    if (slug && size) selectedSizes[slug] = size;
  });
});
