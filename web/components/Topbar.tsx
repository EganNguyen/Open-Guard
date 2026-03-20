import styles from "./Topbar.module.css";

interface TopbarProps {
  title: string;
}

export default function Topbar({ title }: TopbarProps) {
  return (
    <header className={styles.topbar}>
      <span className={styles.pageTitle}>{title}</span>
      <div className={styles.spacer} />

      {/* Search */}
      <div className={styles.searchBox}>
        <svg width="13" height="13" viewBox="0 0 20 20" fill="currentColor" style={{ color: "var(--muted)", flexShrink: 0 }}>
          <path fillRule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8zM2 8a6 6 0 1110.89 3.476l4.817 4.817a1 1 0 01-1.414 1.414l-4.816-4.816A6 6 0 012 8z"/>
        </svg>
        <input type="text" placeholder="Search..." />
      </div>

      {/* Trial badge */}
      <div className={styles.trialBadge}>
        <svg width="11" height="11" viewBox="0 0 20 20" fill="currentColor">
          <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"/>
        </svg>
        Trial ends Apr 19
      </div>

      {/* Notification */}
      <button className={`${styles.topbarBtn} ${styles.notifBtn}`} aria-label="Notifications">
        <div className={styles.notifDot} />
        <svg width="14" height="14" viewBox="0 0 20 20" fill="currentColor">
          <path d="M10 2a6 6 0 00-6 6v3.586l-.707.707A1 1 0 004 14h12a1 1 0 00.707-1.707L16 11.586V8a6 6 0 00-6-6zM10 18a3 3 0 01-3-3h6a3 3 0 01-3 3z"/>
        </svg>
      </button>

      {/* Info */}
      <button className={styles.topbarBtn} aria-label="Help">
        <svg width="14" height="14" viewBox="0 0 20 20" fill="currentColor">
          <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z"/>
        </svg>
      </button>
    </header>
  );
}
