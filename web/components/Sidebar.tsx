"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import styles from "./Sidebar.module.css";

/* ── SVG Icons (inline for zero dependency) ── */
const ShieldIcon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
    <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
  </svg>
);

interface NavItemProps {
  href: string;
  label: string;
  icon: React.ReactNode;
  badge?: string;
  badgeColor?: string;
  active?: boolean;
}

function NavItem({ href, label, icon, badge, badgeColor, active }: NavItemProps) {
  return (
    <Link href={href} className={`${styles.navItem} ${active ? styles.active : ""}`}>
      <span className={styles.navIcon}>{icon}</span>
      {label}
      {badge && (
        <span className={styles.navBadge} style={badgeColor ? { background: "transparent", border: `1px solid ${badgeColor}`, color: badgeColor, fontSize: "8.5px" } : {}}>
          {badge}
        </span>
      )}
    </Link>
  );
}

export default function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className={styles.sidebar}>
      {/* Logo */}
      <div className={styles.logo}>
        <div className={styles.logoMark}>
          <ShieldIcon />
        </div>
        <span className={styles.logoText}>Open<span>Guard</span></span>
      </div>

      {/* Org Switcher */}
      <div className={styles.orgSwitcher}>
        <div className={styles.orgAvatar}>A</div>
        <span className={styles.orgName}>Acme Corp</span>
        <span className={styles.orgCaret}>⌄</span>
      </div>

      {/* Navigation */}
      <nav className={styles.nav}>
        <div className={styles.navSection}>
          <div className={styles.navLabel}>Overview</div>
          <NavItem href="/dashboard" label="Overview" active={pathname === "/dashboard"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path d="M10 2a8 8 0 100 16A8 8 0 0010 2zm0 2a6 6 0 110 12A6 6 0 0110 4zm0 2a1 1 0 100 2 1 1 0 000-2zm1 5a1 1 0 11-2 0V9a1 1 0 112 0v4z"/></svg>
          } />
          <NavItem href="/dashboard/guide" label="Security guide" active={pathname === "/dashboard/guide"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path d="M10 2a8 8 0 100 16A8 8 0 0010 2zm0 2a6 6 0 110 12A6 6 0 0110 4zm0 2a1 1 0 100 2 1 1 0 000-2zm1 5a1 1 0 11-2 0V9a1 1 0 112 0v4z"/></svg>
          } />
        </div>

        <div className={styles.navSection}>
          <div className={styles.navLabel}>User security</div>
          <NavItem href="/dashboard/auth-policies" label="Authentication policies" active={pathname === "/dashboard/auth-policies"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path d="M9 12l-2-2 1.4-1.4L9 9.2l4.6-4.6L15 6l-6 6z"/><path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-2 0a6 6 0 11-12 0 6 6 0 0112 0z"/></svg>
          } />
          <NavItem href="/dashboard/external-users" label="External users" active={pathname === "/dashboard/external-users"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path d="M13 6a3 3 0 11-6 0 3 3 0 016 0zM18 8a2 2 0 11-4 0 2 2 0 014 0zM14 15a4 4 0 00-8 0v1h8v-1zM6 8a2 2 0 11-4 0 2 2 0 014 0zM16 18v-1a5.97 5.97 0 00-1.4-3.86A3 3 0 0119 17v1h-3zM4.4 13.14A5.97 5.97 0 003 17v1H0v-1a3 3 0 013.4-2.86z"/></svg>
          } />
          <NavItem href="/dashboard/access-policies" label="Access policies" badge="NEW" badgeColor="var(--amber)" active={pathname === "/dashboard/access-policies"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z"/></svg>
          } />
          <NavItem href="/dashboard/identity-providers" label="Identity providers" active={pathname === "/dashboard/identity-providers"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path d="M10 9a3 3 0 100-6 3 3 0 000 6zm-7 9a7 7 0 1114 0H3z"/></svg>
          } />
        </div>

        <div className={styles.navSection}>
          <div className={styles.navLabel}>Data protection</div>
          <NavItem href="/dashboard/classification" label="Data classification" active={pathname === "/dashboard/classification"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path d="M4 4a2 2 0 012-2h8a2 2 0 012 2v12l-6-3-6 3V4z"/></svg>
          } />
          <NavItem href="/dashboard/data-security" label="Data security policy" active={pathname === "/dashboard/data-security"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M2.166 4.999A11.954 11.954 0 0010 1.944 11.954 11.954 0 0017.834 5c.11.65.166 1.32.166 2.001 0 5.225-3.34 9.67-8 11.317C5.34 16.67 2 12.225 2 7c0-.682.057-1.35.166-2.001zm11.541 3.708a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"/></svg>
          } />
          <NavItem href="/dashboard/encryption" label="Encryption" active={pathname === "/dashboard/encryption"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M18 8a6 6 0 01-7.743 5.743L10 14l-1 1-1 1H6v2H2v-4l4.257-4.257A6 6 0 1118 8zm-6-4a1 1 0 100 2 2 2 0 012 2 1 1 0 102 0 4 4 0 00-4-4z"/></svg>
          } />
          <NavItem href="/dashboard/hipaa" label="HIPAA compliance" active={pathname === "/dashboard/hipaa"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M6 2a2 2 0 00-2 2v12a2 2 0 002 2h8a2 2 0 002-2V7.414A2 2 0 0015.414 6L12 2.586A2 2 0 0010.586 2H6zm2 10a1 1 0 10-2 0v3a1 1 0 102 0v-3zm2-3a1 1 0 011 1v5a1 1 0 11-2 0v-5a1 1 0 011-1zm4-1a1 1 0 10-2 0v6a1 1 0 102 0V8z"/></svg>
          } />
        </div>

        <div className={styles.navSection}>
          <div className={styles.navLabel}>Detection</div>
          <NavItem href="/dashboard/threats" label="Threat detection" badge="3" active={pathname === "/dashboard/threats"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path d="M10 2a6 6 0 00-6 6v3.586l-.707.707A1 1 0 004 14h12a1 1 0 00.707-1.707L16 11.586V8a6 6 0 00-6-6zM10 18a3 3 0 01-3-3h6a3 3 0 01-3 3z"/></svg>
          } />
          <NavItem href="/dashboard/audit" label="Audit log" active={pathname === "/dashboard/audit"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M3 3a1 1 0 000 2v8a2 2 0 002 2h2.586l-1.293 1.293a1 1 0 101.414 1.414L10 15.414l2.293 2.293a1 1 0 001.414-1.414L12.414 15H15a2 2 0 002-2V5a1 1 0 100-2H3z"/></svg>
          } />
        </div>

        <div className={styles.navSection}>
          <div className={styles.navLabel}>Reports</div>
          <NavItem href="/dashboard/compliance" label="Compliance" active={pathname === "/dashboard/compliance"} icon={
            <svg viewBox="0 0 20 20" fill="currentColor"><path d="M2 11a1 1 0 011-1h2a1 1 0 011 1v5a1 1 0 01-1 1H3a1 1 0 01-1-1v-5zM8 7a1 1 0 011-1h2a1 1 0 011 1v9a1 1 0 01-1 1H9a1 1 0 01-1-1V7zM14 4a1 1 0 011-1h2a1 1 0 011 1v12a1 1 0 01-1 1h-2a1 1 0 01-1-1V4z"/></svg>
          } />
        </div>
      </nav>

      {/* Footer */}
      <div className={styles.footer}>
        <div className={styles.userAvatar}>JD</div>
        <div className={styles.userInfo}>
          <div className={styles.userName}>Jane Doe</div>
          <div className={styles.userRole}>Org Admin</div>
        </div>
        <svg width="14" height="14" viewBox="0 0 20 20" fill="currentColor" style={{ color: "var(--muted)", flexShrink: 0 }}>
          <path d="M6 10a2 2 0 11-4 0 2 2 0 014 0zM12 10a2 2 0 11-4 0 2 2 0 014 0zM16 12a2 2 0 100-4 2 2 0 000 4z"/>
        </svg>
      </div>
    </aside>
  );
}
