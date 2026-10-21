'use strict';
function secureRandomInt(max) {
  const limit = Math.floor(0x100000000 / max) * max;
  const word = new Uint32Array(1);
  let value;
  do { crypto.getRandomValues(word); value = word[0]; } while (value >= limit);
  return value % max;
}
function shuffleCards() {
  const grid = document.querySelector('.collection-grid');
  if (!grid) return;
  const cards = Array.from(grid.children);
  const original = cards.map(card => card.dataset.bookmarkId).join(',');
  for (let i = cards.length - 1; i > 0; i--) {
    const j = secureRandomInt(i + 1);
    [cards[i], cards[j]] = [cards[j], cards[i]];
  }
  if (cards.length > 1 && cards.map(card => card.dataset.bookmarkId).join(',') === original) cards.push(cards.shift());
  cards.forEach(card => grid.appendChild(card));
  grid.dataset.order = cards.map(card => card.dataset.bookmarkId).join(',');
}
function applyCover(card) {
  const image = card.querySelector('.collection-cover-image');
  const filename = card.dataset.bookmarkImage || '';
  if (image && /^[a-z0-9-]+\\.jpg$/i.test(filename)) {
    image.src = `/tool/_assets/covers/photos/${encodeURIComponent(filename)}`;
  }
}
shuffleCards();
document.querySelectorAll('.collection-card').forEach(applyCover);
document.querySelectorAll('.collection-favicon img').forEach(image => {
  image.addEventListener('error', () => image.dataset.error = '1', { once: true });
});
window.__collectionOrder = () => document.querySelector('.collection-grid')?.dataset.order || '';
window.__reshuffleCollection = shuffleCards;
