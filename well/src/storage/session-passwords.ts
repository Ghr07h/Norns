const passwords = new Map<string, string>();

export const sessionPasswords = {
  get(serverId: string): string | undefined {
    return passwords.get(serverId);
  },
  set(serverId: string, password: string): void {
    passwords.set(serverId, password);
  },
  clear(serverId: string): void {
    passwords.delete(serverId);
  },
  clearAll(): void {
    passwords.clear();
  },
};
