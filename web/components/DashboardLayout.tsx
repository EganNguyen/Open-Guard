import Sidebar from "./Sidebar";
import Topbar from "./Topbar";
import styles from "./DashboardLayout.module.css";

interface DashboardLayoutProps {
  title: string;
  children: React.ReactNode;
}

export default function DashboardLayout({ title, children }: DashboardLayoutProps) {
  return (
    <div style={{ display: "flex" }}>
      <Sidebar />
      <main className={styles.main}>
        <Topbar title={title} />
        <div className={styles.content}>
          {children}
        </div>
      </main>
    </div>
  );
}
