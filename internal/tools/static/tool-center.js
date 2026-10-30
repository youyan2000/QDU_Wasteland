'use strict';
document.querySelectorAll('.icon-fill[data-icon]').forEach(element => {
  const draw = window.ICONS && (ICONS[element.dataset.icon] || ICONS.layers || ICONS.book);
  if (draw) element.innerHTML = draw(22);
});
const filters = document.querySelectorAll('[data-filter]');
const cards = document.querySelectorAll('.tool-card[data-status]');
filters.forEach(button => {
  button.addEventListener('click', () => {
    const filter = button.dataset.filter;
    filters.forEach(item => {
      const selected = item === button;
      item.classList.toggle('active', selected);
      item.setAttribute('aria-pressed', String(selected));
    });
    cards.forEach(card => { card.hidden = filter !== 'all' && card.dataset.status !== filter; });
  });
});
