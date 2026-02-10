// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @dev Represents a Cosmos SDK Coin.
struct Coin {
    string denom;
    uint256 amount;
}

/// @dev The LockupI contract's address.
address constant LOCKUP_PRECOMPILE_ADDRESS = 0x0000000000000000000000000000000000000900;

/// @dev The LockupI contract's instance.
LockupI constant LOCKUP_CONTRACT = LockupI(LOCKUP_PRECOMPILE_ADDRESS);

/// @dev Define all the available lockup methods.
string constant MSG_LOCK = "/lockup.v1.MsgLock";
string constant MSG_EXTEND = "/lockup.v1.MsgExtend";
string constant MSG_SEND_DELEGATE_AND_LOCK = "/lockup.v1.MsgSendDelegateAndLock";

/// @dev Represents a lock extension request.
struct LockExtension {
    string fromDate;
    string toDate;
    Coin amount;
}

/// @dev Represents a single lock entry for a user.
struct LockInfo {
    string unlockDate;
    string denom;
    uint256 amount;
}

/// @author TrustedSmartChain Team
/// @title Lockup Precompiled Contract
/// @dev The interface through which solidity contracts will interact with the lockup module.
/// @custom:address 0x0000000000000000000000000000000000000900
interface LockupI {
    /// @dev Lock tokens for a specific address until an unlock date.
    /// @param lockAddress The address whose tokens will be locked
    /// @param unlockDate The date when the tokens will be unlocked (YYYY-MM-DD format)
    /// @param amount The amount of tokens to lock (in bond denomination)
    /// @return success Whether or not the lock was successful
    function lock(
        address lockAddress,
        string memory unlockDate,
        uint256 amount
    ) external returns (bool success);

    /// @dev Extend existing lock durations.
    /// @param lockAddress The address whose lock will be extended
    /// @param extensions An array of LockExtension structs defining the extensions
    /// @return success Whether or not the extension was successful
    function extend(
        address lockAddress,
        LockExtension[] calldata extensions
    ) external returns (bool success);

    /// @dev Send tokens to an address, delegate them to a validator, and lock them.
    /// @param toAddress The recipient address
    /// @param validatorAddress The validator to delegate tokens to (bech32 format)
    /// @param unlockDate The date when the tokens will be unlocked (YYYY-MM-DD format)
    /// @param amount The amount of tokens to send, delegate, and lock
    /// @return success Whether or not the operation was successful
    function sendDelegateAndLock(
        address toAddress,
        string memory validatorAddress,
        string memory unlockDate,
        uint256 amount
    ) external returns (bool success);

    /// @dev Query locks for a specific address.
    /// @param lockAddress The address to query locks for
    /// @return locks An array of LockInfo representing the active locks
    function locks(
        address lockAddress
    ) external view returns (LockInfo[] memory locks);

    /// @dev Query the total locked amount across all accounts.
    /// @return denom The bond denomination
    /// @return totalLocked The total amount of tokens locked
    function totalLockedAmount()
        external
        view
        returns (string memory denom, uint256 totalLocked);

    /// @dev Lock defines an Event emitted when tokens are locked.
    /// @param lockAddress The address of the account locking tokens
    /// @param unlockDate The unlock date for the lock
    /// @param amount The amount of tokens locked
    event Lock(
        address indexed lockAddress,
        string unlockDate,
        uint256 amount
    );

    /// @dev LockExtended defines an Event emitted when a lock is extended.
    /// @param lockAddress The address of the account extending the lock
    /// @param oldUnlockDate The original unlock date
    /// @param newUnlockDate The new unlock date
    /// @param amount The amount of tokens in the extended lock
    event LockExtended(
        address indexed lockAddress,
        string oldUnlockDate,
        string newUnlockDate,
        uint256 amount
    );

    /// @dev SendDelegateAndLock defines an Event emitted when tokens are sent, delegated, and locked.
    /// @param fromAddress The sender address
    /// @param toAddress The recipient address
    /// @param validatorAddress The validator the tokens are delegated to
    /// @param unlockDate The unlock date for the lock
    /// @param amount The amount of tokens
    event SendDelegateAndLock(
        address indexed fromAddress,
        address indexed toAddress,
        string validatorAddress,
        string unlockDate,
        uint256 amount
    );
}
